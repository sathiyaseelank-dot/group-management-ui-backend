import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
} from '@/components/ui/tooltip'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getAgents,
  getAgentDiscoveredServices,
  getResources,
  addResource,
  addResourcesBatch,
  dismissDiscoveredService,
  undismissDiscoveredService,
  getDiscoverySummary,
  purgeDiscoveredServices,
} from '@/lib/mock-api'
import type { Agent, AgentDiscoveredService, Resource, DiscoverySummary } from '@/lib/types'
import { formatServicePort, smartResourceName } from '@/lib/well-known-ports'

const LAST_VISITED_KEY = 'agent-discovery-last-visited'

function timeAgo(unix: number): string {
  if (!unix) return 'never'
  const seconds = Math.floor(Date.now() / 1000 - unix)
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

function StatusDot({ serviceStatus, agentOnline, lastSeen }: { serviceStatus: string; agentOnline: boolean; lastSeen: number }) {
  if (serviceStatus === 'gone') {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex items-center gap-1.5">
            <span className="inline-block h-2 w-2 rounded-full bg-red-500" />
            <span className="text-xs text-red-600">Gone</span>
          </span>
        </TooltipTrigger>
        <TooltipContent>Service no longer detected. Last seen {timeAgo(lastSeen)}</TooltipContent>
      </Tooltip>
    )
  }

  if (serviceStatus === 'stale') {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex items-center gap-1.5">
            <span className="inline-block h-2 w-2 rounded-full bg-yellow-500" />
            <span className="text-xs text-yellow-600">Stale</span>
          </span>
        </TooltipTrigger>
        <TooltipContent>Agent offline, service state unknown. Last seen {timeAgo(lastSeen)}</TooltipContent>
      </Tooltip>
    )
  }

  if (!agentOnline) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex items-center gap-1.5">
            <span className="inline-block h-2 w-2 rounded-full bg-gray-400" />
            <span className="text-xs text-gray-500">Agent Offline</span>
          </span>
        </TooltipTrigger>
        <TooltipContent>Agent is offline. Last known state: active. Last seen {timeAgo(lastSeen)}</TooltipContent>
      </Tooltip>
    )
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center gap-1.5">
          <span className="inline-block h-2 w-2 rounded-full bg-green-500" />
          <span className="text-xs text-green-600">Active</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>Service is running. Last seen {timeAgo(lastSeen)}</TooltipContent>
    </Tooltip>
  )
}

function isWildcard(ip: string): boolean {
  return !ip || ip === '0.0.0.0' || ip === '::' || ip === '::0' || ip === '0:0:0:0:0:0:0:0'
}

function resolveAddress(svc: AgentDiscoveredService, agent: Agent | undefined): string {
  if (!isWildcard(svc.boundIp)) return svc.boundIp
  return agent?.hostname || '0.0.0.0'
}

function isManaged(svc: AgentDiscoveredService, resources: Resource[]): boolean {
  return resources.some(
    (r) =>
      r.portFrom === svc.port &&
      r.protocol.toLowerCase() === svc.protocol.toLowerCase() &&
      (r.address === svc.boundIp || r.address === '0.0.0.0' || svc.boundIp === '0.0.0.0' || svc.boundIp === '')
  )
}

interface AgentGroup {
  agent: Agent | undefined
  agentId: string
  services: AgentDiscoveredService[]
  activeCount: number
  goneCount: number
  staleCount: number
  agentOffline: boolean
}

function SummaryCards({ summary }: { summary: DiscoverySummary | null }) {
  if (!summary) return null
  const cards = [
    { label: 'Active Services', value: summary.total, color: 'text-foreground' },
    { label: 'New (24h)', value: summary.new_24h, color: 'text-blue-600' },
    { label: 'Unmanaged', value: summary.unmanaged, color: 'text-orange-600' },
    { label: 'Gone', value: summary.gone, color: 'text-red-600' },
    { label: 'Stale', value: summary.stale, color: 'text-yellow-600' },
  ]
  return (
    <div className="grid grid-cols-5 gap-4">
      {cards.map((c) => (
        <div key={c.label} className="rounded-md border p-4">
          <p className="text-xs text-muted-foreground">{c.label}</p>
          <p className={`text-2xl font-bold ${c.color}`}>{c.value}</p>
        </div>
      ))}
    </div>
  )
}

export default function AgentDiscoveryPage() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgent, setSelectedAgent] = useState('__all__')
  const [services, setServices] = useState<AgentDiscoveredService[]>([])
  const [resources, setResources] = useState<Resource[]>([])
  const [summary, setSummary] = useState<DiscoverySummary | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const [addedServices, setAddedServices] = useState<Set<number>>(new Set())
  const [showDismissed, setShowDismissed] = useState(false)
  const [lastVisited, setLastVisited] = useState<number>(0)
  const [expandedAgents, setExpandedAgents] = useState<Set<string>>(new Set())
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkAdding, setBulkAdding] = useState(false)
  const [purging, setPurging] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Load last visited timestamp on mount
  useEffect(() => {
    const stored = localStorage.getItem(LAST_VISITED_KEY)
    if (stored) setLastVisited(parseInt(stored, 10))

    return () => {
      localStorage.setItem(LAST_VISITED_KEY, String(Math.floor(Date.now() / 1000)))
    }
  }, [])

  useEffect(() => {
    getAgents().then(setAgents).catch(() => {})
    getResources().then(setResources).catch(() => {})
    getDiscoverySummary().then(setSummary).catch(() => {})
  }, [])

  const agentIdParam = (val: string) => (val === '__all__' ? undefined : val)

  const fetchResults = useCallback(async (agentId?: string) => {
    setLoading(true)
    setError(null)
    try {
      const results = await getAgentDiscoveredServices(agentId, showDismissed)
      setServices(results)
      setLastRefresh(new Date())
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [showDismissed])

  useEffect(() => {
    fetchResults(agentIdParam(selectedAgent))

    intervalRef.current = setInterval(() => {
      fetchResults(agentIdParam(selectedAgent))
      getDiscoverySummary().then(setSummary).catch(() => {})
    }, 15000)

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [selectedAgent, fetchResults])

  // Group services by agent
  const agentGroups = useMemo((): AgentGroup[] => {
    const map = new Map<string, AgentDiscoveredService[]>()
    for (const svc of services) {
      const list = map.get(svc.agentId) || []
      list.push(svc)
      map.set(svc.agentId, list)
    }

    const groups: AgentGroup[] = []
    for (const [agentId, svcs] of map) {
      const agent = agents.find((a) => a.id === agentId)
      let activeCount = 0
      let goneCount = 0
      let staleCount = 0
      for (const s of svcs) {
        if (s.status === 'gone') goneCount++
        else if (s.status === 'stale') staleCount++
        else activeCount++
      }
      const agentOffline = agent?.status !== 'online'
      groups.push({ agent, agentId, services: svcs, activeCount, goneCount, staleCount, agentOffline })
    }
    return groups.sort((a, b) => (a.agent?.name || a.agentId).localeCompare(b.agent?.name || b.agentId))
  }, [services, agents])

  // Auto-expand all agents
  useEffect(() => {
    setExpandedAgents(new Set(agentGroups.map((g) => g.agentId)))
  }, [agentGroups])

  const handleAddResource = async (svc: AgentDiscoveredService) => {
    try {
      const agent = agents.find((a) => a.id === svc.agentId)
      const networkId = agent?.remoteNetworkId
      if (!networkId) {
        setError('No remote network found for this agent. Assign the agent to a remote network first.')
        return
      }
      const name = smartResourceName(svc.port, agent?.name || svc.agentId)
      await addResource({
        network_id: networkId,
        name,
        type: 'STANDARD',
        address: resolveAddress(svc, agent),
        protocol: svc.protocol.toUpperCase() as 'TCP' | 'UDP',
        port_from: svc.port,
        port_to: svc.port,
      })
      setAddedServices((prev) => new Set([...prev, svc.id]))
      getResources().then(setResources).catch(() => {})
      getDiscoverySummary().then(setSummary).catch(() => {})
    } catch (err) {
      setError(`Failed to add resource: ${(err as Error).message}`)
    }
  }

  const handleBulkAdd = async () => {
    if (selected.size === 0) return
    setBulkAdding(true)
    setError(null)

    const toAdd = services.filter((s) => selected.has(s.id) && !isManaged(s, resources) && !addedServices.has(s.id) && s.status !== 'gone')
    if (toAdd.length === 0) {
      setError('No eligible services selected')
      setBulkAdding(false)
      return
    }

    // Group by agent to resolve network IDs
    const byAgent = new Map<string, AgentDiscoveredService[]>()
    for (const svc of toAdd) {
      const list = byAgent.get(svc.agentId) || []
      list.push(svc)
      byAgent.set(svc.agentId, list)
    }

    let totalCreated = 0
    const allErrors: string[] = []

    for (const [agentId, svcs] of byAgent) {
      const agent = agents.find((a) => a.id === agentId)
      const networkId = agent?.remoteNetworkId
      if (!networkId) {
        allErrors.push(`Agent ${agent?.name || agentId} has no remote network`)
        continue
      }

      try {
        const result = await addResourcesBatch(
          svcs.map((svc) => ({
            network_id: networkId,
            name: smartResourceName(svc.port, agent?.name || agentId),
            type: 'STANDARD' as const,
            address: resolveAddress(svc, agent),
            protocol: svc.protocol.toUpperCase() as 'TCP' | 'UDP',
            port_from: svc.port,
            port_to: svc.port,
          }))
        )
        totalCreated += result.created
        if (result.errors?.length) allErrors.push(...result.errors)
      } catch (err) {
        allErrors.push((err as Error).message)
      }
    }

    if (totalCreated > 0) {
      setAddedServices((prev) => {
        const next = new Set(prev)
        toAdd.forEach((s) => next.add(s.id))
        return next
      })
      getResources().then(setResources).catch(() => {})
      getDiscoverySummary().then(setSummary).catch(() => {})
    }

    if (allErrors.length > 0) {
      setError(`Added ${totalCreated} resources. Errors: ${allErrors.join('; ')}`)
    }

    setSelected(new Set())
    setBulkAdding(false)
  }

  const handleDismiss = async (svc: AgentDiscoveredService) => {
    try {
      await dismissDiscoveredService(svc.id)
      setServices((prev) => prev.filter((s) => s.id !== svc.id))
      getDiscoverySummary().then(setSummary).catch(() => {})
    } catch (err) {
      setError(`Failed to dismiss: ${(err as Error).message}`)
    }
  }

  const handleUndismiss = async (svc: AgentDiscoveredService) => {
    try {
      await undismissDiscoveredService(svc.id)
      setServices((prev) => prev.map((s) => (s.id === svc.id ? { ...s, dismissed: false } : s)))
    } catch (err) {
      setError(`Failed to undismiss: ${(err as Error).message}`)
    }
  }

  const handlePurge = async () => {
    if (!window.confirm('Are you sure you want to delete all discovered services? This cannot be undone.')) return
    setPurging(true)
    setError(null)
    try {
      const agentId = agentIdParam(selectedAgent)
      await purgeDiscoveredServices(agentId)
      setServices([])
      setSelected(new Set())
      setAddedServices(new Set())
      getDiscoverySummary().then(setSummary).catch(() => {})
    } catch (err) {
      setError(`Failed to purge: ${(err as Error).message}`)
    } finally {
      setPurging(false)
    }
  }

  const toggleAgent = (agentId: string) => {
    setExpandedAgents((prev) => {
      const next = new Set(prev)
      if (next.has(agentId)) next.delete(agentId)
      else next.add(agentId)
      return next
    })
  }

  const toggleSelect = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleSelectAll = (svcs: AgentDiscoveredService[]) => {
    const eligibleIds = svcs.filter((s) => !isManaged(s, resources) && !addedServices.has(s.id) && s.status !== 'gone' && !s.dismissed).map((s) => s.id)
    const allSelected = eligibleIds.every((id) => selected.has(id))
    setSelected((prev) => {
      const next = new Set(prev)
      if (allSelected) {
        eligibleIds.forEach((id) => next.delete(id))
      } else {
        eligibleIds.forEach((id) => next.add(id))
      }
      return next
    })
  }

  const isNew = (svc: AgentDiscoveredService) => lastVisited > 0 && svc.firstSeen > lastVisited

  const hasContent = !!summary

  return (
    <div className={`space-y-6 p-6${hasContent ? ' min-h-full bg-background' : ''}`}>
      <div>
        <h1 className="text-2xl font-bold">Agent Discovery</h1>
        <p className="text-muted-foreground">
          Services automatically discovered on agent LANs
        </p>
      </div>

      <SummaryCards summary={summary} />

      {error && (
        <div className="rounded-md bg-red-50 p-4 text-sm text-red-800">{error}</div>
      )}

      <div className="flex items-center gap-4">
        <Select value={selectedAgent} onValueChange={setSelectedAgent}>
          <SelectTrigger className="w-64">
            <SelectValue placeholder="All agents" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All agents</SelectItem>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name || a.id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button
          variant="outline"
          onClick={() => fetchResults(agentIdParam(selectedAgent))}
          disabled={loading}
        >
          {loading ? 'Refreshing...' : 'Refresh'}
        </Button>

        <Button
          variant="destructive"
          size="sm"
          onClick={handlePurge}
          disabled={purging || services.length === 0}
        >
          {purging ? 'Clearing...' : 'Clear All'}
        </Button>

        <label className="flex items-center gap-2 text-sm text-muted-foreground cursor-pointer">
          <input
            type="checkbox"
            checked={showDismissed}
            onChange={(e) => setShowDismissed(e.target.checked)}
            className="rounded"
          />
          Show dismissed
        </label>

        {lastRefresh && (
          <span className="text-xs text-muted-foreground ml-auto">
            Last refreshed: {lastRefresh.toLocaleTimeString()}
          </span>
        )}
      </div>

      {/* Floating bulk action bar */}
      {selected.size > 0 && (
        <div className="sticky top-0 z-10 flex items-center gap-4 rounded-md border bg-card p-3 shadow-md">
          <span className="text-sm font-medium">{selected.size} selected</span>
          <Button size="sm" onClick={handleBulkAdd} disabled={bulkAdding}>
            {bulkAdding ? 'Adding...' : `Add ${selected.size} as Resources`}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
            Clear
          </Button>
        </div>
      )}

      {services.length === 0 && !loading ? (
        <div className="rounded-md border border-dashed p-8 text-center text-muted-foreground">
          No services discovered yet. Agents report new LAN services every 30 seconds.
        </div>
      ) : (
        <div className="space-y-4">
          {agentGroups.map((group) => (
            <Collapsible
              key={group.agentId}
              open={expandedAgents.has(group.agentId)}
              onOpenChange={() => toggleAgent(group.agentId)}
            >
              <div className="rounded-md border">
                <CollapsibleTrigger asChild>
                  <button className="flex w-full items-center justify-between p-4 text-left hover:bg-muted/50 transition-colors">
                    <div className="flex items-center gap-3">
                      <span className="text-sm font-medium">
                        {expandedAgents.has(group.agentId) ? '\u25BC' : '\u25B6'}
                      </span>
                      <div>
                        <span className="font-semibold">
                          {group.agent?.name || group.agentId}
                        </span>
                        {group.agent?.hostname && (
                          <span className="ml-2 text-xs text-muted-foreground">
                            ({group.agent.hostname})
                          </span>
                        )}
                      </div>
                    </div>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground">
                      {group.agentOffline ? (
                        <>
                          <span className="text-gray-500">Agent Offline</span>
                          <span>{group.services.length} services</span>
                          {group.goneCount > 0 && (
                            <span className="text-red-500">{group.goneCount} gone</span>
                          )}
                          {group.staleCount > 0 && (
                            <span className="text-yellow-500">{group.staleCount} stale</span>
                          )}
                        </>
                      ) : (
                        <>
                          {group.activeCount > 0 && (
                            <span className="text-green-600">{group.activeCount} active</span>
                          )}
                          {group.goneCount > 0 && (
                            <span className="text-red-500">{group.goneCount} gone</span>
                          )}
                          {group.staleCount > 0 && (
                            <span className="text-yellow-500">{group.staleCount} stale</span>
                          )}
                        </>
                      )}
                    </div>
                  </button>
                </CollapsibleTrigger>

                <CollapsibleContent>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-10">
                          <input
                            type="checkbox"
                            className="rounded"
                            checked={
                              group.services.filter((s) => !isManaged(s, resources) && !addedServices.has(s.id) && s.status !== 'gone' && !s.dismissed).length > 0 &&
                              group.services.filter((s) => !isManaged(s, resources) && !addedServices.has(s.id) && s.status !== 'gone' && !s.dismissed).every((s) => selected.has(s.id))
                            }
                            onChange={() => toggleSelectAll(group.services)}
                          />
                        </TableHead>
                        <TableHead>Status</TableHead>
                        <TableHead>Service / Port</TableHead>
                        <TableHead>Protocol</TableHead>
                        <TableHead>Bound IP</TableHead>
                        <TableHead>First Seen</TableHead>
                        <TableHead>Last Seen</TableHead>
                        <TableHead className="text-right">Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {group.services.map((svc) => {
                        const managed = isManaged(svc, resources)
                        const isNewService = isNew(svc)
                        const isGone = svc.status === 'gone'
                        const canSelect = !managed && !addedServices.has(svc.id) && !isGone && !svc.dismissed

                        return (
                          <TableRow
                            key={svc.id}
                            className={svc.dismissed ? 'opacity-50' : isGone ? 'opacity-70' : undefined}
                          >
                            <TableCell>
                              {canSelect ? (
                                <input
                                  type="checkbox"
                                  className="rounded"
                                  checked={selected.has(svc.id)}
                                  onChange={() => toggleSelect(svc.id)}
                                />
                              ) : (
                                <span className="inline-block w-4" />
                              )}
                            </TableCell>
                            <TableCell>
                              <div className="flex items-center gap-2">
                                <StatusDot serviceStatus={svc.status} agentOnline={group.agent?.status === 'online'} lastSeen={svc.lastSeen} />
                                {isNewService && (
                                  <Badge variant="default" className="bg-blue-500 text-[10px] px-1.5 py-0">
                                    NEW
                                  </Badge>
                                )}
                              </div>
                            </TableCell>
                            <TableCell className="font-mono text-sm">
                              <div className="flex flex-col">
                                <span>{svc.serviceName || formatServicePort(svc.port)}</span>
                                {svc.processName && (
                                  <span className="text-xs text-muted-foreground">{svc.processName} :{svc.port}</span>
                                )}
                                {!svc.processName && svc.serviceName && (
                                  <span className="text-xs text-muted-foreground">:{svc.port}</span>
                                )}
                              </div>
                            </TableCell>
                            <TableCell className="uppercase text-xs">{svc.protocol}</TableCell>
                            <TableCell className="font-mono text-xs">{svc.boundIp || '0.0.0.0'}</TableCell>
                            <TableCell className="text-xs">
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span>{timeAgo(svc.firstSeen)}</span>
                                </TooltipTrigger>
                                <TooltipContent>
                                  {new Date(svc.firstSeen * 1000).toLocaleString()}
                                </TooltipContent>
                              </Tooltip>
                            </TableCell>
                            <TableCell className="text-xs">{timeAgo(svc.lastSeen)}</TableCell>
                            <TableCell>
                              <div className="flex items-center justify-end gap-2">
                                {managed || addedServices.has(svc.id) ? (
                                  <Badge variant="secondary" className="text-green-700 bg-green-50 border-green-200">
                                    Managed
                                  </Badge>
                                ) : isGone ? (
                                  <Badge variant="secondary" className="text-red-700 bg-red-50 border-red-200">
                                    Gone
                                  </Badge>
                                ) : (
                                  <Button
                                    size="sm"
                                    variant="outline"
                                    onClick={() => handleAddResource(svc)}
                                  >
                                    Add as Resource
                                  </Button>
                                )}
                                {svc.dismissed ? (
                                  <Button
                                    size="sm"
                                    variant="ghost"
                                    onClick={() => handleUndismiss(svc)}
                                    className="text-xs"
                                  >
                                    Restore
                                  </Button>
                                ) : (
                                  <Button
                                    size="sm"
                                    variant="ghost"
                                    onClick={() => handleDismiss(svc)}
                                    className="text-xs text-muted-foreground"
                                  >
                                    Dismiss
                                  </Button>
                                )}
                              </div>
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </CollapsibleContent>
              </div>
            </Collapsible>
          ))}
        </div>
      )}
    </div>
  )
}
