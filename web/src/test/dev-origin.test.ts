// @vitest-environment node
import {
  createServer as createHTTPServer,
  request,
  type Server,
} from 'node:http'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { createServer, type ViteDevServer } from 'vite'
import { canonicalDevOrigin } from '../../dev-origin'

describe.each(['127.0.0.1', 'localhost'])('canonical dev host %s', (host) => {
  let server: ViteDevServer
  let http: Server
  let port: number

  beforeAll(async () => {
    server = await createServer({
      configFile: false,
      plugins: [
        canonicalDevOrigin(`http://${host}:5173`),
        {
          name: 'test-endpoint',
          configureServer(vite) {
            vite.middlewares.use((_request, response) => {
              response.statusCode = 204
              response.end()
            })
          },
        },
      ],
      optimizeDeps: { noDiscovery: true, include: [] },
      server: { middlewareMode: true, hmr: false, watch: null },
    })
    http = createHTTPServer(server.middlewares)
    await new Promise<void>((resolve) => {
      http.listen(0, '127.0.0.1', resolve)
    })
    const address = http.address()
    if (!address || typeof address === 'string') throw new Error('No test port')
    port = address.port
  })

  afterAll(async () => {
    await server?.close()
    await new Promise<void>((resolve, reject) => {
      http?.close((error) => (error ? reject(error) : resolve()))
    })
  })

  function navigate(
    requestHost: string,
    path = '/setup?from=dev',
    method = 'GET',
    accept = 'text/html',
    origin?: string,
  ) {
    return new Promise<{ status: number; location?: string }>(
      (resolve, reject) => {
        const req = request(
          {
            hostname: '127.0.0.1',
            port,
            path,
            method,
            headers: {
              Host: requestHost,
              Accept: accept,
              ...(origin ? { Origin: origin } : {}),
            },
          },
          (response) => {
            response.resume()
            response.on('end', () =>
              resolve({
                status: response.statusCode!,
                location: response.headers.location,
              }),
            )
          },
        )
        req.on('error', reject)
        req.end()
      },
    )
  }

  it('redirects loopback navigation to the configured origin, retaining path and query', async () => {
    for (const alias of ['localhost', '127.0.0.1', '[::1]']) {
      if (alias === host) continue
      expect(await navigate(`${alias}:5173`)).toEqual({
        status: 307,
        location: `http://${host}:5173/setup?from=dev`,
      })
    }
    expect(await navigate(`${host}:5173`)).toEqual({ status: 204 })
  })

  it('does not redirect API calls, mutations, assets or other ports', async () => {
    const alias = host === 'localhost' ? '127.0.0.1' : 'localhost'
    for (const [requestHost, path, method, accept] of [
      [`${alias}:5173`, '/api/v1/setup', 'POST', 'text/html'],
      [`${alias}:5173`, '/api/v1/setup/status', 'GET', 'text/html'],
      [`${alias}:5173`, '/healthz', 'GET', 'text/html'],
      [`${alias}:5173`, '/setup', 'GET', '*/*'],
      [`${alias}:8080`, '/setup', 'GET', 'text/html'],
    ] as const) {
      expect(await navigate(requestHost, path, method, accept)).toEqual({
        status: 204,
      })
    }
  })

  it('never redirects cross-origin API mutations to conceal their Origin', async () => {
    expect(
      await navigate(
        'localhost:5173',
        '/api/v1/setup',
        'POST',
        'text/html',
        'https://evil.example',
      ),
    ).toEqual({ status: 204 })
  })
})
