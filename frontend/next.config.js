/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  reactStrictMode: true,
  transpilePackages: [],
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: "http://localhost:8080/api/:path*",
      },
      {
        source: "/scim/:path*",
        destination: "http://localhost:8080/scim/:path*",
      },
      {
        source: "/graphql",
        destination: "http://localhost:8080/graphql",
      },
    ]
  },
}

module.exports = nextConfig
