import type { NextConfig } from "next"

const nextConfig: NextConfig = {
  reactStrictMode: true,
  transpilePackages: ["@tremor/react"],
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

export default nextConfig
