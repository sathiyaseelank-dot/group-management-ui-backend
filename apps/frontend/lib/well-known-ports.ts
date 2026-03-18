const WELL_KNOWN_PORTS: Record<number, string> = {
  21: 'FTP',
  22: 'SSH',
  25: 'SMTP',
  53: 'DNS',
  80: 'HTTP',
  110: 'POP3',
  143: 'IMAP',
  443: 'HTTPS',
  465: 'SMTPS',
  587: 'SMTP',
  993: 'IMAPS',
  995: 'POP3S',
  1433: 'MSSQL',
  1521: 'Oracle',
  2375: 'Docker',
  2376: 'Docker TLS',
  3000: 'Dev Server',
  3306: 'MySQL',
  3389: 'RDP',
  4222: 'NATS',
  5432: 'PostgreSQL',
  5672: 'RabbitMQ',
  6379: 'Redis',
  6443: 'Kubernetes API',
  8080: 'HTTP Proxy',
  8081: 'HTTP Alt',
  8443: 'gRPC/TLS',
  8888: 'HTTP Alt',
  9090: 'Prometheus',
  9200: 'Elasticsearch',
  9443: 'Connector',
  11211: 'Memcached',
  15672: 'RabbitMQ Mgmt',
  27017: 'MongoDB',
}

export function getPortLabel(port: number): string | undefined {
  return WELL_KNOWN_PORTS[port]
}

export function formatServicePort(port: number): string {
  const label = WELL_KNOWN_PORTS[port]
  return label ? `${label} :${port}` : `:${port}`
}

export function smartResourceName(port: number, agentName: string): string {
  const label = WELL_KNOWN_PORTS[port]
  if (label) {
    return `${label} on ${agentName} (:${port})`
  }
  return `Service on ${agentName} (:${port})`
}
