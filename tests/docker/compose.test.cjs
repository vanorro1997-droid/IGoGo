const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { after, test } = require('node:test');

const root = path.resolve(__dirname, '../..');
const compose = fs.readFileSync(path.join(root, 'docker-compose.yml'), 'utf8');
const dockerfile = fs.readFileSync(path.join(root, 'Dockerfile'), 'utf8');
const temporary = fs.mkdtempSync(path.join(process.env.IGOGO_TEST_TEMP_DIR || os.tmpdir(), 'igogo-compose-test-'));
const envFile = path.join(temporary, 'test.env');
fs.writeFileSync(envFile, 'DB_ENABLED=true\nDB_PASSWORD=compose-test-only-password\n');
after(function () { fs.rmSync(temporary, { recursive: true, force: true }); });

const defaultImage = 'public.ecr.aws/docker/library/postgres:15.20-alpine3.23@sha256:83bdbdd781076b1543709883a93d995b53e682b078ea30e3cea012856732d1e5';
const buildImages = {
  GO_IMAGE: 'public.ecr.aws/docker/library/golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc',
  ALPINE_IMAGE: 'public.ecr.aws/docker/library/alpine:3.23.8@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2',
};

function configuration(overrides = {}) {
  const env = { ...process.env };
  const variables = Array.from(compose.matchAll(/\$\{([A-Z_][A-Z0-9_]*)/g), function (match) { return match[1]; });
  for (const key of [...variables, 'COMPOSE_FILE', 'COMPOSE_ENV_FILES', 'COMPOSE_DISABLE_ENV_FILE']) {
    delete env[key];
  }
  Object.assign(env, overrides);
  const result = spawnSync('docker', [
    'compose', '-f', path.join(root, 'docker-compose.yml'),
    '--env-file', envFile, 'config', '--format', 'json',
  ], { cwd: root, env, encoding: 'utf8' });
  assert.equal(result.error, undefined, 'Docker CLI must be installed to run configuration tests');
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

test('Compose defaults to the verified PostgreSQL 15.20 image and immutable index digest', function () {
  assert.equal(configuration().services.postgres.image, defaultImage);
});

test('Dockerfile and Compose use matching pinned builder and runtime images', function () {
  const defaults = Object.fromEntries(Array.from(dockerfile.matchAll(/^ARG (GO_IMAGE|ALPINE_IMAGE)=(.+)$/gm), function (match) {
    return [match[1], match[2].trim()];
  }));
  assert.deepEqual(defaults, buildImages);
  assert.deepEqual(configuration().services.app.build.args, buildImages);
  assert.match(dockerfile, /^FROM \$\{GO_IMAGE\} AS builder\r?$/m);
  assert.match(dockerfile, /^FROM \$\{ALPINE_IMAGE\}\r?$/m);
});

test('empty image overrides still use the defaults', function () {
  const config = configuration({ POSTGRES_IMAGE: '', GO_IMAGE: '', ALPINE_IMAGE: '' });
  assert.equal(config.services.postgres.image, defaultImage);
  assert.deepEqual(config.services.app.build.args, buildImages);
});

test('POSTGRES_IMAGE allows a different registry without editing Compose', function () {
  const customImage = defaultImage.replace('public.ecr.aws/docker', 'registry.example.test');
  assert.equal(configuration({ POSTGRES_IMAGE: customImage }).services.postgres.image, customImage);
});

test('builder and runtime registries can be overridden without editing Dockerfile', function () {
  const customImages = Object.fromEntries(Object.entries(buildImages).map(function ([key, image]) {
    return [key, image.replace('public.ecr.aws/docker', 'registry.example.test')];
  }));
  assert.deepEqual(configuration(customImages).services.app.build.args, customImages);
});

test('Compose does not force AMD64 or another architecture', function () {
  const config = configuration();
  assert.equal(config.services.postgres.platform, undefined);
  assert.equal(config.services.app.platform, undefined);
  assert.doesNotMatch(dockerfile, /(?:GOARCH|--platform)\s*=\s*["']?(?:linux\/)?[a-z0-9]+/i);
});

test('the image change preserves the database volume and healthy-start dependency', function () {
  const config = configuration();
  assert.ok(config.services.postgres.volumes.some(function (volume) {
    return volume.type === 'volume' && volume.source === 'postgres_data' && volume.target === '/var/lib/postgresql/data';
  }));
  assert.equal(config.services.app.depends_on.postgres.condition, 'service_healthy');
  assert.equal(config.services.app.environment.DB_HOST, 'postgres');
  assert.equal(config.services.app.environment.DB_PORT, '5432');
});
