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
  // 'gift' adds amount to the wallet; 'discount' (or '' in older rows)
  // takes percent off.
  kind: string
  amount: number
  planIds: string
  kinds: '' | 'buy' | 'renew'
  oncePerUser: boolean
}

export interface ShopSms {
  id: number
  sender: string
  text: string
  amount: number
  orderId: number
  status: 'approved' | 'unmatched' | 'ignored' | 'duplicate' | 'error'
  note: string
  createdAt: number
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
  // extra makes a card payment's amount unique: the customer transfers
  // paid + extra, and extra goes to the wallet.
  extra: number
  card: string
  auto: boolean
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
  sms: ShopSms[]
  settings: Record<string, string>
}
