import type { NextConfig } from 'next';

// This is a frontend proxy only. The Go service is not changed.
const backendUrl = (process.env.BACKEND_URL || 'http://127.0.0.1:18080').replace(/\/$/, '');
const nextConfig: NextConfig = {
  async rewrites() {
    return [{ source: '/api/:path*', destination: `${backendUrl}/:path*` }];
  },
};
export default nextConfig;
