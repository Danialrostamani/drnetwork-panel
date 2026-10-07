// What api/shop answers: the Sales page.

export interface ShopPlan {
  id: number
  name: string
  volume: number
  days: number
  limitIp: number
  price: number
  group: string
  enable: boolean
  sort: number
}

export interface ShopDiscount {
  id: number
  code: string
  percent: number
  maxUses: number
  used: number
  expiry: number
  enable: boolean
}

export interface ShopOrder {
  id: number
  tgId: number
  tgName: string
  kind: 'buy' | 'renew' | 'topup'
  planId: number
  planName: string
  clientId: number
  clientName: string
  volume: number
  days: number
  amount: number
  discount: number
  paid: number
  code: string
  method: string
  receipt: string
  status: 'pending' | 'approved' | 'rejected' | 'canceled'
  createdAt: number
  decidedAt: number
  reseller: boolean
}

export interface ShopStats {
  currency: string
  revenueToday: number
  revenue7: number
  revenue30: number
  revenueAll: number
  orders30: number
  renewals30: number
  pending: number
  users: number
  newUsers30: number
  trials: number
  walletTotal: number
  clients: number
  activeClients: number
  online24h: number
  expiring3d: number
  expired: number
  depleted: number
  days: { day: string, revenue: number, orders: number }[]
  topConsumers: { name: string, usage: number }[]
  topPlans: { name: string, orders: number, revenue: number }[]
}

export interface ShopData {
  stats: ShopStats
  plans: ShopPlan[]
  discounts: ShopDiscount[]
  orders: ShopOrder[]
  settings: Record<string, string>
}
