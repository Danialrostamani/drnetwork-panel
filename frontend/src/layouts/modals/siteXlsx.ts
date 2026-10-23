// A small, dependency-free Excel (.xlsx) writer for the site report: one
// sheet with a title block, a styled table and a totals row.
import type { SiteRow } from './siteStats'

export interface SheetLabels {
  title: string
  user: string
  from: string
  to: string
  sites: string
  rank: string
  site: string
  download: string
  upload: string
  total: string
  share: string
  readable: string
  sum: string
}

const MB = 1048576

// ---- zip (stored, no compression) ----

const crcTable = (() => {
  const t = new Uint32Array(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    t[n] = c >>> 0
  }
  return t
})()

export function crc32(data: Uint8Array): number {
  let c = 0xffffffff
  for (const b of data) c = crcTable[(c ^ b) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

export function zip(files: { name: string; data: string }[]): Uint8Array {
  const enc = new TextEncoder()
  const parts: Uint8Array[] = []
  const central: Uint8Array[] = []
  let offset = 0
  // 1980-01-01 00:00 in DOS time; the date carries no meaning here.
  const dosDate = (0 << 9) | (1 << 5) | 1
  for (const f of files) {
    const name = enc.encode(f.name)
    const data = enc.encode(f.data)
    const crc = crc32(data)
    const local = new DataView(new ArrayBuffer(30))
    local.setUint32(0, 0x04034b50, true)
    local.setUint16(4, 20, true)
    local.setUint16(6, 0x0800, true) // UTF-8 names
    local.setUint16(8, 0, true)
    local.setUint16(10, 0, true)
    local.setUint16(12, dosDate, true)
    local.setUint32(14, crc, true)
    local.setUint32(18, data.length, true)
    local.setUint32(22, data.length, true)
    local.setUint16(26, name.length, true)
    local.setUint16(28, 0, true)
    parts.push(new Uint8Array(local.buffer), name, data)
    const cd = new DataView(new ArrayBuffer(46))
    cd.setUint32(0, 0x02014b50, true)
    cd.setUint16(4, 20, true)
    cd.setUint16(6, 20, true)
    cd.setUint16(8, 0x0800, true)
    cd.setUint16(10, 0, true)
    cd.setUint16(12, 0, true)
    cd.setUint16(14, dosDate, true)
    cd.setUint32(16, crc, true)
    cd.setUint32(20, data.length, true)
    cd.setUint32(24, data.length, true)
    cd.setUint16(28, name.length, true)
    cd.setUint32(42, offset, true)
    central.push(new Uint8Array(cd.buffer), name)
    offset += 30 + name.length + data.length
  }
  const cdSize = central.reduce((a, p) => a + p.length, 0)
  const end = new DataView(new ArrayBuffer(22))
  end.setUint32(0, 0x06054b50, true)
  end.setUint16(8, files.length, true)
  end.setUint16(10, files.length, true)
  end.setUint32(12, cdSize, true)
  end.setUint32(16, offset, true)
  const all = [...parts, ...central, new Uint8Array(end.buffer)]
  const out = new Uint8Array(all.reduce((a, p) => a + p.length, 0))
  let at = 0
  for (const p of all) {
    out.set(p, at)
    at += p.length
  }
  return out
}

// ---- sheet ----

export function xmlText(s: string): string {
  return s
    // eslint-disable-next-line no-control-regex
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
}

function col(i: number): string {
  return String.fromCharCode(65 + i)
}

// Style indexes, see styles below.
const S = { title: 1, label: 2, header: 3, text: 4, int: 5, num: 6, pct: 7, sumText: 8, sumNum: 9, sumPct: 10, value: 11 }

const styles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<fonts count="4"><font><sz val="11"/><name val="Tahoma"/></font><font><b/><sz val="14"/><name val="Tahoma"/></font><font><b/><sz val="11"/><name val="Tahoma"/></font><font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="Tahoma"/></font></fonts>
<fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF2F5597"/><bgColor indexed="64"/></patternFill></fill><fill><patternFill patternType="solid"><fgColor rgb="FFDDEBF7"/><bgColor indexed="64"/></patternFill></fill></fills>
<borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color rgb="FFBFBFBF"/></left><right style="thin"><color rgb="FFBFBFBF"/></right><top style="thin"><color rgb="FFBFBFBF"/></top><bottom style="thin"><color rgb="FFBFBFBF"/></bottom><diagonal/></border></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="12">
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment horizontal="center" vertical="center"/></xf>
<xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="0" fontId="3" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment horizontal="center" vertical="center" wrapText="1"/></xf>
<xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1"/>
<xf numFmtId="1" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="center"/></xf>
<xf numFmtId="4" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/>
<xf numFmtId="10" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/>
<xf numFmtId="0" fontId="2" fillId="3" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/>
<xf numFmtId="4" fontId="2" fillId="3" borderId="1" xfId="0" applyNumberFormat="1" applyFont="1" applyFill="1" applyBorder="1"/>
<xf numFmtId="10" fontId="2" fillId="3" borderId="1" xfId="0" applyNumberFormat="1" applyFont="1" applyFill="1" applyBorder="1"/>
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment horizontal="left"/></xf>
</cellXfs>
<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`

type Cell = { s: number; v: string | number } | null

function cellXml(ref: string, c: Cell): string {
  if (!c) return ''
  if (typeof c.v === 'number') return `<c r="${ref}" s="${c.s}"><v>${Number.isFinite(c.v) ? c.v : 0}</v></c>`
  return `<c r="${ref}" s="${c.s}" t="inlineStr"><is><t xml:space="preserve">${xmlText(c.v)}</t></is></c>`
}

const round2 = (v: number) => Math.round(v * 100) / 100

// siteSheet is the worksheet XML: the report window on top, then the table
// (frozen header, filter) and a totals row.
export function siteSheet(l: SheetLabels, user: string, from: string, to: string, rows: SiteRow[], readable: (v: number) => string, rtl: boolean): string {
  const sum = rows.reduce((a, r) => ({ up: a.up + r.up, down: a.down + r.down }), { up: 0, down: 0 })
  const grand = sum.up + sum.down
  const grid: Cell[][] = [
    [{ s: S.title, v: l.title }],
    [null, { s: S.label, v: l.user }, { s: S.value, v: user }],
    [null, { s: S.label, v: l.from }, { s: S.value, v: from }],
    [null, { s: S.label, v: l.to }, { s: S.value, v: to }],
    [null, { s: S.label, v: l.sites }, { s: S.value, v: rows.length }],
    [],
    [l.rank, l.site, l.download, l.upload, l.total, l.share, l.readable].map(v => ({ s: S.header, v })),
  ]
  const head = grid.length
  rows.forEach((r, i) => grid.push([
    { s: S.int, v: i + 1 },
    { s: S.text, v: r.site },
    { s: S.num, v: round2(r.down / MB) },
    { s: S.num, v: round2(r.up / MB) },
    { s: S.num, v: round2(r.total / MB) },
    { s: S.pct, v: grand ? r.total / grand : 0 },
    { s: S.text, v: readable(r.total) },
  ]))
  grid.push([
    { s: S.sumText, v: '' },
    { s: S.sumText, v: l.sum },
    { s: S.sumNum, v: round2(sum.down / MB) },
    { s: S.sumNum, v: round2(sum.up / MB) },
    { s: S.sumNum, v: round2(grand / MB) },
    { s: S.sumPct, v: grand ? 1 : 0 },
    { s: S.sumText, v: readable(grand) },
  ])
  const sheetRows = grid.map((cells, i) => {
    const r = i + 1
    const ht = i === head - 1 ? ' ht="22" customHeight="1"' : i === 0 ? ' ht="26" customHeight="1"' : ''
    return `<row r="${r}"${ht}>${cells.map((c, j) => cellXml(col(j) + r, c)).join('')}</row>`
  }).join('')
  const widths = [6, 42, 16, 16, 16, 12, 16]
  const lastData = head + rows.length
  return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheetPr><pageSetUpPr fitToPage="1"/></sheetPr>
<sheetViews><sheetView workbookViewId="0"${rtl ? ' rightToLeft="1"' : ''}><pane ySplit="${head}" topLeftCell="A${head + 1}" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>
<sheetFormatPr defaultRowHeight="16"/>
<cols>${widths.map((w, i) => `<col min="${i + 1}" max="${i + 1}" width="${w}" customWidth="1"/>`).join('')}</cols>
<sheetData>${sheetRows}</sheetData>
${rows.length ? `<autoFilter ref="A${head}:G${lastData}"/>` : ''}
<mergeCells count="5"><mergeCell ref="A1:G1"/><mergeCell ref="C2:G2"/><mergeCell ref="C3:G3"/><mergeCell ref="C4:G4"/><mergeCell ref="C5:G5"/></mergeCells>
<printOptions horizontalCentered="1"/>
<pageMargins left="0.5" right="0.5" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>
<pageSetup orientation="portrait" fitToWidth="1" fitToHeight="0"/>
</worksheet>`
}

// siteXlsx is the whole workbook, ready for a Blob.
export function siteXlsx(sheetName: string, sheet: string): Uint8Array {
  const name = xmlText(sheetName.replace(/[\\/?*[\]:]/g, ' ').slice(0, 31) || 'Sites')
  return zip([
    { name: '[Content_Types].xml', data: `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>` },
    { name: '_rels/.rels', data: `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>` },
    { name: 'xl/workbook.xml', data: `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="${name}" sheetId="1" r:id="rId1"/></sheets></workbook>` },
    { name: 'xl/_rels/workbook.xml.rels', data: `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>` },
    { name: 'xl/styles.xml', data: styles },
    { name: 'xl/worksheets/sheet1.xml', data: sheet },
  ])
}
