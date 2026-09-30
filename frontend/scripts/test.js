const { execSync } = require('child_process');
const path = require('path');

const rootDir = path.join(__dirname, '..');

console.log('Running frontend TypeScript type check (tsc --noEmit)...');
execSync('npx tsc --noEmit', { cwd: rootDir, stdio: 'inherit' });

console.log('Running frontend unit tests...');
execSync('node --test __tests__/*.test.js', { cwd: rootDir, stdio: 'inherit' });

console.log('All frontend tests passed cleanly!');
