import type { NextConfig } from 'next';

// Preserve canonical API paths and proxy the frontend compatibility/Auth routes.
const backendUrl = (process.env.BACKEND_URL || 'http://127.0.0.1:18080').replace(/\/$/, '');
const nextConfig: NextConfig = {
  devIndicators: false,
  async rewrites() {
    return [
      { source: '/health', destination: `${backendUrl}/health` },
      { source: '/ready', destination: `${backendUrl}/ready` },
      { source: '/api/v1/:path*', destination: `${backendUrl}/api/v1/:path*` },
      { source: '/api/:path*', destination: `${backendUrl}/:path*` },
    ];
  },
};
export default nextConfig;
