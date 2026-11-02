import type { Plugin } from 'vite'

export function canonicalDevOrigin(baseURL: string): Plugin {
  const canonical = new URL(baseURL)
  const loopbackHosts = ['localhost', '127.0.0.1', '[::1]']
  const aliases = loopbackHosts.map(
    (host) => `${host}${canonical.port ? `:${canonical.port}` : ''}`,
  )

  return {
    name: 'canonical-dev-origin',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (
          canonical.protocol === 'http:' &&
          loopbackHosts.includes(canonical.hostname) &&
          request.headers.host !== canonical.host &&
          aliases.includes(request.headers.host ?? '') &&
          (request.method === 'GET' || request.method === 'HEAD') &&
          request.headers.accept?.includes('text/html') &&
          !request.url?.startsWith('/api/') &&
          request.url !== '/healthz'
        ) {
          response.writeHead(307, {
            Location: canonical.origin + (request.url ?? '/'),
            'Cache-Control': 'no-store',
          })
          response.end()
          return
        }
        next()
      })
    },
  }
}
