import { useState, useEffect, useRef } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getConnectors,
  startNetworkScan,
  getScanStatus,
  addResource,
} from '@/lib/mock-api'
import type { Connector, ScanJob, DiscoveredResource } from '@/lib/types'

export default function NetworkDiscoveryPage() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [selectedConnector, setSelectedConnector] = useState('')
  const [cidrInput, setCidrInput] = useState('192.168.1.0/24')
  const [portsInput, setPortsInput] = useState('22,80,443,3389,8080')
  const [scanning, setScanning] = useState(false)
  const [scanJob, setScanJob] = useState<ScanJob | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [addedResources, setAddedResources] = useState<Set<string>>(new Set())
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    getConnectors().then((list) => {
      const online = list.filter(
        (c) => c.status === 'online'
      )
      setConnectors(online)
    })
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [])

  const handleStartScan = async () => {
    setError(null)
    setScanJob(null)
    setAddedResources(new Set())

    if (!selectedConnector) {
      setError('Please select a connector')
      return
    }
    if (!cidrInput.trim()) {
      setError('Please enter at least one CIDR range')
      return
    }

    const targets = cidrInput
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    const ports = portsInput
      .split(',')
      .map((s) => parseInt(s.trim(), 10))
      .filter((n) => !isNaN(n) && n > 0 && n <= 65535)

    if (ports.length === 0) {
      setError('Please enter at least one valid port')
      return
    }

    setScanning(true)
    try {
      const result = await startNetworkScan(selectedConnector, targets, ports)
      const requestId = result.request_id

      // Start polling
      setScanJob({
        requestId,
        connectorId: selectedConnector,
        status: 'pending',
        targets,
        ports,
        startedAt: new Date().toISOString(),
      })

      pollRef.current = setInterval(async () => {
        try {
          const status = await getScanStatus(requestId)
          setScanJob(status)
          if (
            status.status === 'completed' ||
            status.status === 'failed'
          ) {
            if (pollRef.current) {
              clearInterval(pollRef.current)
              pollRef.current = null
            }
            setScanning(false)
          }
        } catch {
          // keep polling
        }
      }, 2000)
    } catch (err) {
      setError((err as Error).message)
      setScanning(false)
    }
  }

  const handleAddResource = async (resource: DiscoveredResource) => {
    try {
      const connectorId = resource.reachableFrom || selectedConnector
      const connector = connectors.find((c) => c.id === connectorId)
      const networkId = connector?.remoteNetworkId
      if (!networkId) {
        setError('No remote network found for this connector. Assign the connector to a remote network first.')
        return
      }
      await addResource({
        network_id: networkId,
        name: `${resource.serviceName && resource.serviceName !== 'Unknown' ? resource.serviceName + '@' : ''}${resource.ip}:${resource.port}`,
        type: 'STANDARD',
        address: resource.ip,
        protocol: 'TCP',
        port_from: resource.port,
        port_to: resource.port,
      })
      setAddedResources((prev) => new Set([...prev, resource.id]))
    } catch (err) {
      setError(`Failed to add resource: ${(err as Error).message}`)
    }
  }

  const statusBadge = (status: string) => {
    const colors: Record<string, string> = {
      pending: 'bg-yellow-100 text-yellow-800',
      in_progress: 'bg-blue-100 text-blue-800',
      completed: 'bg-green-100 text-green-800',
      failed: 'bg-red-100 text-red-800',
    }
    return (
      <span
        className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colors[status] || 'bg-gray-100 text-gray-800'}`}
      >
        {status.replace('_', ' ')}
      </span>
    )
  }

  const hasContent = !!scanJob

  return (
    <div className={`space-y-6 p-6${hasContent ? ' min-h-full bg-background' : ''}`}>
      <div>
        <h1 className="text-2xl font-bold">Network Discovery</h1>
        <p className="text-muted-foreground">
          Scan connector networks to discover reachable resources
        </p>
      </div>

      {/* Scan Form */}
      <div className="rounded-lg border p-6 space-y-4">
        <h2 className="text-lg font-semibold">Start Scan</h2>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Connector</label>
            <Select
              value={selectedConnector}
              onValueChange={setSelectedConnector}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select an online connector" />
              </SelectTrigger>
              <SelectContent>
                {connectors.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name || c.id} ({c.privateIp || 'unknown IP'})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">
              CIDR Ranges (comma-separated)
            </label>
            <Input
              placeholder="192.168.1.0/24, 10.0.0.0/24"
              value={cidrInput}
              onChange={(e) => setCidrInput(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium">
              Ports (comma-separated)
            </label>
            <Input
              placeholder="22,80,443,3389,8080"
              value={portsInput}
              onChange={(e) => setPortsInput(e.target.value)}
            />
          </div>

          <div className="flex items-end">
            <Button
              onClick={handleStartScan}
              disabled={scanning}
              className="w-full"
            >
              {scanning ? 'Scanning...' : 'Start Scan'}
            </Button>
          </div>
        </div>

        {error && (
          <div className="rounded-md bg-red-50 p-3 text-sm text-red-700">
            {error}
          </div>
        )}
      </div>

      {/* Scan Status */}
      {scanJob && (
        <div className="rounded-lg border p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold">Scan Status</h2>
            {statusBadge(scanJob.status)}
          </div>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
            <div>
              <span className="text-muted-foreground">Request ID:</span>
              <p className="font-mono text-xs">{scanJob.requestId}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Connector:</span>
              <p>{scanJob.connectorId}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Targets:</span>
              <p>{scanJob.targets?.join(', ')}</p>
            </div>
            <div>
              <span className="text-muted-foreground">Ports:</span>
              <p>{scanJob.ports?.join(', ')}</p>
            </div>
          </div>
          {scanJob.error && (
            <div className="rounded-md bg-red-50 p-3 text-sm text-red-700">
              {scanJob.error}
            </div>
          )}
        </div>
      )}

      {/* Results Table */}
      {scanJob?.status === 'completed' && scanJob.results && (
        <div className="rounded-lg border">
          <div className="p-4 border-b">
            <h2 className="text-lg font-semibold">
              Discovered Resources ({scanJob.results.length})
            </h2>
          </div>
          {scanJob.results.length === 0 ? (
            <div className="p-6 text-center text-muted-foreground">
              No resources discovered in the scanned range.
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>IP</TableHead>
                  <TableHead>Port</TableHead>
                  <TableHead>Service</TableHead>
                  <TableHead>Protocol</TableHead>
                  <TableHead>Connector</TableHead>
                  <TableHead>First Seen</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {scanJob.results.map((resource) => (
                  <TableRow key={resource.id}>
                    <TableCell className="font-mono">
                      {resource.ip}
                    </TableCell>
                    <TableCell>{resource.port}</TableCell>
                    <TableCell>
                      <span className="inline-flex items-center rounded-full bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700">
                        {resource.serviceName || 'Unknown'}
                      </span>
                    </TableCell>
                    <TableCell className="uppercase">
                      {resource.protocol}
                    </TableCell>
                    <TableCell>{resource.reachableFrom}</TableCell>
                    <TableCell>
                      {new Date(resource.firstSeen * 1000).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant={
                          addedResources.has(resource.id)
                            ? 'secondary'
                            : 'default'
                        }
                        disabled={addedResources.has(resource.id)}
                        onClick={() => handleAddResource(resource)}
                      >
                        {addedResources.has(resource.id)
                          ? 'Added'
                          : 'Add as Resource'}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      )}
    </div>
  )
}
