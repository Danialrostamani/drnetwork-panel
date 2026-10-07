import {
  Chart as ChartJS, BarElement, CategoryScale, Filler, Legend, LinearScale, LineElement, PointElement, Tooltip,
  type ChartOptions,
} from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, Tooltip, Legend, Filler)
ChartJS.defaults.font.family = 'Vazirmatn'

export interface ChartLook {
  text: string
  grid: string
}

export const chartColors = {
  cpu: '#2196F3',
  mem: '#9C27B0',
  disk: '#FF9800',
  latency: '#00ACC1',
  users: '#43A047',
  up: '#E91E63',
  down: '#3F51B5',
  uptime: '#43A047',
}

type Format = (v: number) => string

// What the tooltip callbacks read of an item, whatever the chart type.
interface LabelItem {
  raw: unknown
  dataset: { label?: string }
}

function label(format?: Format) {
  return (item: LabelItem) => {
    const raw = item.raw as number | null
    return `${item.dataset.label}: ${raw == null ? '-' : format ? format(raw) : String(raw)}`
  }
}

// Options of a time line. Null values are gaps, not zeros.
export function lineOptions(look: ChartLook, format?: Format, max?: number): ChartOptions<'line'> {
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    interaction: { intersect: false, mode: 'index' },
    elements: { point: { radius: 0, hitRadius: 8 }, line: { tension: 0.25, borderWidth: 2 } },
    plugins: {
      legend: { labels: { color: look.text, boxWidth: 12 } },
      tooltip: { callbacks: { label: label(format) } },
    },
    scales: {
      x: { ticks: { color: look.text, maxTicksLimit: 8, maxRotation: 0 }, grid: { color: look.grid } },
      y: {
        beginAtZero: true,
        max,
        ticks: { color: look.text, callback: v => (format ? format(Number(v)) : v) },
        grid: { color: look.grid },
      },
    },
  }
}

export function barOptions(look: ChartLook, format?: Format): ChartOptions<'bar'> {
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    interaction: { intersect: false, mode: 'index' },
    plugins: {
      legend: { labels: { color: look.text, boxWidth: 12 } },
      tooltip: { callbacks: { label: label(format) } },
    },
    scales: {
      x: { ticks: { color: look.text, maxRotation: 0, autoSkip: true }, grid: { color: look.grid } },
      y: {
        beginAtZero: true,
        ticks: { color: look.text, callback: v => (format ? format(Number(v)) : v) },
        grid: { color: look.grid },
      },
    },
  }
}
