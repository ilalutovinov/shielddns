/** @type {import('next').NextConfig} */
module.exports = {
  output: 'standalone',
  typescript: { ignoreBuildErrors: true },
  eslint: { ignoreDuringBuilds: true },
  // В dev и без Caddy запросы /api/* проксируются на Go-API. В проде /api/* обслуживает Caddy.
  async rewrites() {
    return [{ source: '/api/:path*', destination: `${process.env.API_INTERNAL_URL || 'http://localhost:8080'}/api/:path*` }]
  },
}
