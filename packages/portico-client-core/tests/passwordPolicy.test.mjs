import test from "node:test";
import assert from "node:assert/strict";
import { evaluatePorticoPassword, PORTICO_PASSWORD_POLICY, utf8ByteLength } from "../dist/passwordPolicy.js";

test("portable password policy matches the server creation boundary", () => {
  assert.equal(PORTICO_PASSWORD_POLICY.minimumCharacters, 8);
  assert.equal(PORTICO_PASSWORD_POLICY.maximumUtf8Bytes, 72);
  assert.equal(evaluatePorticoPassword("Portico1").valid, true);
  assert.equal(evaluatePorticoPassword("Portico!").valid, true);
  assert.equal(evaluatePorticoPassword("portico1").valid, false);
  assert.equal(evaluatePorticoPassword("PORTICO1").valid, false);
  assert.equal(evaluatePorticoPassword("PorticoPass").valid, false);
  assert.equal(evaluatePorticoPassword("Port1!").valid, false);
  assert.equal(evaluatePorticoPassword(`A1${"x".repeat(70)}`).valid, true);
  assert.equal(evaluatePorticoPassword(`A1${"x".repeat(71)}`).valid, false);
});

test("UTF-8 byte length is portable across BMP and supplementary code points", () => {
  assert.equal(utf8ByteLength("Portico"), 7);
  assert.equal(utf8ByteLength("é"), 2);
  assert.equal(utf8ByteLength("€"), 3);
  assert.equal(utf8ByteLength("😀"), 4);
});

test("strength is informational and rewards materially longer varied passwords", () => {
  assert.deepEqual(evaluatePorticoPassword("Portico1").strength, "weak");
  assert.deepEqual(evaluatePorticoPassword("Portico-Password1").strength, "medium");
  const strong = evaluatePorticoPassword("A considerably longer Portico passphrase 42!");
  assert.equal(strong.valid, true);
  assert.equal(strong.strength, "strong");
});
