import { describe, expect, it } from 'vitest'
import { parseWgConf, WgConfError } from './wgConf'

const sample = `[Interface]
PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=
Address = 10.2.0.2/32, fd00::2/128
DNS = 10.2.0.1
MTU = 1280

# Proton NL
[Peer]
PublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=
PresharedKey = /UwcSPg38hW/D9Y3tcS1FOV0K1wuURMbS0sesJEP5ak=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = nl.example.net:51820
PersistentKeepalive = 25
`

describe('parseWgConf', () => {
  it('reads the interface and the peer', () => {
    const c = parseWgConf(sample)
    expect(c.private_key).toBe('yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=')
    expect(c.address).toEqual(['10.2.0.2/32', 'fd00::2/128'])
    expect(c.mtu).toBe(1280)
    expect(c.dns).toEqual(['10.2.0.1'])
    expect(c.peers).toEqual([{
      public_key: 'xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=',
      pre_shared_key: '/UwcSPg38hW/D9Y3tcS1FOV0K1wuURMbS0sesJEP5ak=',
      allowed_ips: ['0.0.0.0/0', '::/0'],
      address: 'nl.example.net',
      port: 51820,
      persistent_keepalive_interval: 25,
    }])
  })

  it('takes IPv6 endpoints, WARP reserved bytes and CRLF lines', () => {
    const c = parseWgConf('[Interface]\r\nPrivateKey = k=\r\nAddress = 172.16.0.2/32\r\n[Peer]\r\nPublicKey = p=\r\nEndpoint = [2606:4700:d0::a29f:c001]:2408\r\nReserved = AQID\r\n')
    expect(c.peers[0].address).toBe('2606:4700:d0::a29f:c001')
    expect(c.peers[0].port).toBe(2408)
    expect(c.peers[0].reserved).toEqual([1, 2, 3])
    expect(parseWgConf('[Interface]\nPrivateKey=k\nAddress=10.0.0.2/32\n[Peer]\nPublicKey=p\nReserved = [12, 34, 56]').peers[0].reserved).toEqual([12, 34, 56])
  })

  it('keeps a server config, whose peers have no endpoint', () => {
    const c = parseWgConf('[Interface]\nPrivateKey = k\nAddress = 10.0.0.1/24\nListenPort = 51820\n[Peer]\nPublicKey = a\nAllowedIPs = 10.0.0.2/32\n[Peer]\nPublicKey = b\nAllowedIPs = 10.0.0.3/32')
    expect(c.listen_port).toBe(51820)
    expect(c.peers.map(p => p.allowed_ips)).toEqual([['10.0.0.2/32'], ['10.0.0.3/32']])
    expect(c.peers[0].address).toBeUndefined()
  })

  it('says what is missing', () => {
    const why = (t: string) => { try { parseWgConf(t); return '' } catch (e) { return (e as WgConfError).key } }
    expect(why('hello')).toBe('importNoInterface')
    expect(why('[Interface]\nAddress = 10.0.0.2/32')).toBe('importNoKey')
    expect(why('[Interface]\nPrivateKey = k')).toBe('importNoAddress')
    expect(why('[Interface]\nPrivateKey = k\nAddress = 10.0.0.2/32\n[Peer]\nEndpoint = a:1')).toBe('importBadPeer')
    expect(why('[Interface]\nPrivateKey = k\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = p\nEndpoint = nowhere')).toBe('importBadEndpoint')
  })
})
