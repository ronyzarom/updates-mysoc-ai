import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

// Read the repository build version (MAJOR.MINOR.PATCH.BUILD) so the dashboard
// can report an accurate version instead of a hardcoded value.
const __dirname = dirname(fileURLToPath(import.meta.url));
let appVersion = process.env.NEXT_PUBLIC_APP_VERSION || '';
if (!appVersion) {
  try {
    appVersion = readFileSync(join(__dirname, '..', 'VERSION'), 'utf8').trim();
  } catch {
    appVersion = 'dev';
  }
}

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  env: {
    // Default to same-origin (relative) requests. Set NEXT_PUBLIC_API_URL to
    // target a separate API origin (e.g. http://localhost:8080 in local dev).
    NEXT_PUBLIC_API_URL: process.env.NEXT_PUBLIC_API_URL || '',
    NEXT_PUBLIC_APP_VERSION: appVersion,
  },
  async headers() {
    return [
      {
        source: '/:path*',
        headers: [
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'X-Frame-Options', value: 'DENY' },
          { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
          {
            key: 'Permissions-Policy',
            value: 'camera=(), microphone=(), geolocation=()',
          },
        ],
      },
    ];
  },
};

export default nextConfig;
