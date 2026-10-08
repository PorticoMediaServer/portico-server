import test, { type TestContext } from "node:test";
import assert from "node:assert/strict";
import { DeviceAuthorizationService, ApiError } from "../src/index.ts";
// Each test owns its clock. Waiting 10-25 real milliseconds races the second
// scheduled poll against the assertion when other test processes saturate CPU.
async function settle() { for (let n = 0; n < 32; n++) await Promise.resolve(); }
function clock(t: TestContext) {
 t.mock.timers.enable({ apis: ['setTimeout', 'Date'], now: 1800000000000 });
 return async (ms = 1) => { t.mock.timers.tick(ms); await settle(); };
}
const authorization = {
  deviceCode: "private-secret",
  userCode: "ABCD-EFGH",
  verificationUri: "https://example.com/device",
  verificationUriComplete: "https://example.com/device#code=ABCD-EFGH",
  expiresIn: 600,
  expiresAt: new Date(1800000000000 + 600000).toISOString(),
  interval: 0,
};
const session = {
  accessToken: "private-token",
  expiresAt: "later",
  account: { id: "a", username: "a", displayName: "A" },
  profiles: [],
};
const options = { minimumPollMs: 1, requestTimeoutMs: 100 };
test("device secret never enters snapshot and approval is delivered", async (t) => {
  const advance = clock(t);
  let count = 0;
  const service = new DeviceAuthorizationService(
    {
      create: async () => authorization,
      poll: async () => {
        if (++count === 1)
          throw new ApiError(400, "authorization_pending", "Pending");
        return session;
      },
      cancel: async () => {},
      revoke: async () => {},
    },
    options,
    async () => crypto.randomUUID()
  );
  await service.start("TV", "tvos");
  assert.equal(
    JSON.stringify(service.getSnapshot()).includes("private-secret"),
    false
  );
  await advance(); // pending response schedules the next poll
  assert.equal(count, 1);
  await advance();
  assert.equal(service.getSnapshot().phase, "approved");
  assert.equal(count, 2);
  service.cancel();
});
test("cancel fences late token and revokes returned session", async (t) => {
  const advance = clock(t);
  let resolve: any;
  let revoked = 0,
    cancelled = 0;
  const service = new DeviceAuthorizationService(
    {
      create: async () => authorization,
      poll: () => new Promise((r) => (resolve = r)),
      cancel: async () => {
        cancelled++;
      },
      revoke: async () => {
        revoked++;
      },
    },
    options,
    async () => crypto.randomUUID()
  );
  await service.start("TV", "tvos");
  await advance();
  service.cancel();
  resolve(session);
  await advance();
  assert.equal(service.getSnapshot().phase, "idle");
  assert.equal(revoked, 1);
  assert.equal(cancelled, 1);
});
test("late create after local timeout is cancelled", async (t) => {
  const advance = clock(t);
  let resolve: any;
  let cancelled = 0;
  const service = new DeviceAuthorizationService(
    {
      create: () => new Promise((r) => (resolve = r)),
      poll: async () => session,
      cancel: async () => {
        cancelled++;
      },
      revoke: async () => {},
    },
    { minimumPollMs: 1, requestTimeoutMs: 5 },
    async () => crypto.randomUUID()
  );
  const started = service.start("TV", "tvos");
  await settle();
  await advance(5);
  await advance(5);
  await started;
  assert.equal(service.getSnapshot().phase, "error");
  resolve(authorization);
  await advance();
  assert.equal(cancelled, 1);
  service.cancel();
});

test('ambiguous create retries the same private request ID', async (t) => {
  clock(t);
  const ids:string[]=[];
  const service=new DeviceAuthorizationService({create:async(requestId)=>{ids.push(requestId);if(ids.length===1)throw new Error('connection lost');return authorization},poll:async()=>session,cancel:async()=>{},revoke:async()=>{}},options,async()=>crypto.randomUUID());
  await service.start('TV','tvos');assert.equal(ids.length,2);assert.equal(ids[0],ids[1]);service.cancel();
});

test('approval remains cancellable until the host accepts ownership', async(t)=>{
 const advance = clock(t);
 let cancelled=0;
 const service=new DeviceAuthorizationService({create:async()=>authorization,poll:async()=>session,cancel:async()=>{cancelled++},revoke:async()=>{}},options,async()=>crypto.randomUUID());
 await service.start('TV','tvos');await advance();service.cancel();assert.equal(cancelled,1);assert.equal(service.accept(session),false);
 await service.start('TV','tvos');await advance();assert.equal(service.accept(session),true);service.cancel();assert.equal(cancelled,1);
});

test('slow_down backs off and denial is terminal', async(t)=>{
 const advance = clock(t);
 let polls=0;
 const service=new DeviceAuthorizationService({create:async()=>authorization,poll:async()=>{polls++;throw new ApiError(429,'slow_down','Wait',true,10)},cancel:async()=>{},revoke:async()=>{}},options,async()=>crypto.randomUUID());
 await service.start('TV','tvos');await advance();await advance(9999);assert.equal(polls,1);assert.equal(service.getSnapshot().phase,'waiting');service.cancel();
 const denied=new DeviceAuthorizationService({create:async()=>authorization,poll:async()=>{throw new ApiError(400,'access_denied','This request was denied.')},cancel:async()=>{},revoke:async()=>{}},options,async()=>crypto.randomUUID());
 await denied.start('TV','tvos');await advance();assert.equal(denied.getSnapshot().phase,'error');assert.equal(denied.getSnapshot().error,'This request was denied.');denied.cancel();
});
