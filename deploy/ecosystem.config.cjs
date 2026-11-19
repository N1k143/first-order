const path = require('node:path');
const root = path.resolve(__dirname, '..');

module.exports = {
  apps: [
    {
      name: 'ingeneriya-catalog',
      cwd: path.join(root, 'backend'),
      script: path.join(root, 'deploy/run-api.sh'),
      interpreter: '/bin/bash',
      kill_timeout: 12000,
      exp_backoff_restart_delay: 1000,
      time: true,
    },
    {
      name: 'ingeneriya-frontend',
      cwd: path.join(root, 'frontend'),
      script: path.join(root, 'frontend/node_modules/next/dist/bin/next'),
      args: ['start', '-H', '127.0.0.1', '-p', '18080'],
      interpreter: '/usr/bin/node',
      env: { NODE_ENV: 'production' },
      kill_timeout: 12000,
      exp_backoff_restart_delay: 1000,
      time: true,
    },
  ],
};
