/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  images: {
    domains: [
      'localhost',
      'pub-3d8d451a31e06119c646505763f99a2c.r2.dev'
    ],
    remotePatterns: [
      {
        protocol: 'https',
        hostname: '**.r2.dev',
      },
      {
        protocol: 'http',
        hostname: 'localhost',
        port: '8080',
      }
    ]
  }
}

module.exports = nextConfig
