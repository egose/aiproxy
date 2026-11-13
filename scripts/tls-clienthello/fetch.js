const target = new URL(process.argv[2]);
if (
  target.protocol !== 'https:' ||
  target.hostname !== '127.0.0.1' ||
  !target.port ||
  target.username ||
  target.password ||
  target.pathname !== '/' ||
  target.search ||
  target.hash
) {
  throw new Error('capture requires a literal loopback HTTPS URL');
}

console.log(
  JSON.stringify({
    version: Bun.version,
    revision: Bun.revision,
    platform: process.platform,
    arch: process.arch,
  }),
);

try {
  await fetch(target.href, {
    method: 'GET',
    proxy: false,
    redirect: 'error',
    signal: AbortSignal.timeout(4000),
    tls: { serverName: 'localhost' },
  });
  process.exitCode = 1;
} catch {}
