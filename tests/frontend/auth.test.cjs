const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const html = fs.readFileSync(path.join(__dirname, '../../frontend/auth.html'), 'utf8');
const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];

function loadPage(fetch) {
  const elements = new Map(Array.from(html.matchAll(/\bid="([^"]+)"/g), function (match) {
    return [match[1], {
      value: '',
      hidden: false,
      disabled: false,
      textContent: '',
      className: '',
      classList: { toggle() {} },
      listeners: new Map(),
      addEventListener(event, handler) { this.listeners.set(event, handler); },
    }];
  }));
  const tokens = new Map();
  const redirects = [];
  vm.runInNewContext(script, {
    document: { getElementById(id) { return elements.get(id); } },
    localStorage: {
      getItem(key) { return tokens.get(key) || null; },
      setItem(key, value) { tokens.set(key, value); },
      removeItem(key) { tokens.delete(key); },
    },
    location: { assign(url) { redirects.push(url); } },
    fetch,
  }, { filename: 'auth.html' });

  function click(id) {
    return elements.get(id).listeners.get('click')();
  }
  function submit(mode = 'register', email = 'user@example.test', password = 'test-password') {
    click('tab-' + mode);
    elements.get('email').value = email;
    elements.get('password').value = password;
    return elements.get('auth-form').listeners.get('submit')({ preventDefault() {} });
  }
  return { elements, tokens, redirects, click, submit };
}

function response(status, payload) {
  return {
    status,
    ok: status >= 200 && status < 300,
    async text() { return typeof payload === 'string' ? payload : JSON.stringify(payload); },
  };
}

function assertError(page, message, mode = 'register') {
  const status = page.elements.get('status');
  assert.equal(status.hidden, false, 'the error must stay visible after the request finishes');
  assert.equal(status.className, 'status error');
  assert.equal(status.textContent, message);
  assert.equal(page.elements.get('submit').disabled, false);
  assert.equal(page.elements.get('submit').textContent, mode === 'register' ? 'Создать аккаунт' : 'Войти');
  assert.deepEqual(page.redirects, []);
}

for (const [status, message] of [
  [400, 'invalid email'],
  [409, 'email already registered'],
  [500, 'failed to create user'],
  [503, 'database is not configured'],
]) {
  test('registration keeps the HTTP ' + status + ' error visible', async function () {
    const page = loadPage(async function () { return response(status, { error: message }); });
    await page.submit();
    assertError(page, message);
  });
}

test('registration keeps a non-JSON server error visible', async function () {
  const page = loadPage(async function () { return response(502, 'Bad Gateway'); });
  await page.submit();
  assertError(page, 'Bad Gateway');
});

test('login keeps an invalid credentials error visible', async function () {
  const page = loadPage(async function () { return response(401, { error: 'invalid email or password' }); });
  await page.submit('login');
  assertError(page, 'invalid email or password', 'login');
});

test('registration keeps network errors visible and allows retrying', async function () {
  const page = loadPage(async function () { throw new Error('Network request failed'); });
  await page.submit();
  assertError(page, 'Не удалось выполнить запрос: Network request failed');
  page.click('tab-login');
  assert.equal(page.elements.get('status').hidden, true, 'changing tabs should clear old errors');
});

test('registration shows progress while waiting and restores the button afterward', async function () {
  let finish;
  const page = loadPage(function () { return new Promise(function (resolve) { finish = resolve; }); });
  const submitted = page.submit();
  assert.equal(page.elements.get('submit').disabled, true);
  assert.equal(page.elements.get('submit').textContent, 'Создаём аккаунт…');
  assert.equal(page.elements.get('status').hidden, false);
  assert.equal(page.elements.get('status').textContent, 'Отправляем запрос…');
  finish(response(409, { error: 'email already registered' }));
  await submitted;
  assertError(page, 'email already registered');
});

for (const mode of ['register', 'login']) {
  test(mode + ' stores the session token and redirects on success', async function () {
    const requests = [];
    const page = loadPage(async function (url, options) {
      requests.push({ url, method: options.method, body: JSON.parse(options.body) });
      return response(mode === 'register' ? 201 : 200, { token: 'test-session-token' });
    });
    await page.submit(mode);
    assert.deepEqual(requests, [{
      url: '/api/auth/' + mode,
      method: 'POST',
      body: { email: 'user@example.test', password: 'test-password' },
    }]);
    assert.equal(page.tokens.get('igogo_token'), 'test-session-token');
    assert.deepEqual(page.redirects, ['/']);
    assert.equal(page.elements.get('submit').disabled, false);
    assert.equal(page.elements.get('submit').textContent, mode === 'register' ? 'Создать аккаунт' : 'Войти');
  });
}

test('registration validates short passwords without sending a request', async function () {
  const page = loadPage(function () { assert.fail('invalid input must not be submitted'); });
  await page.submit('register', 'user@example.test', 'short');
  assertError(page, 'Пароль должен быть не короче 8 символов.');
});
