import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import ts from 'typescript';

for (const family of ['layout', 'browse']) {
  test(`${family} obtains its own external DEV inputs without importing private HTML or raw responses`, () => {
    const url = new URL(`../src/dev/families/${family}.tsx`, import.meta.url);
    const source = ts.createSourceFile(url.pathname, readFileSync(url, 'utf8'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
    const imports = source.statements.filter(ts.isImportDeclaration);
    for (const declaration of imports) {
      assert.ok(ts.isStringLiteral(declaration.moduleSpecifier));
      const specifier = declaration.moduleSpecifier.text;
      assert.doesNotMatch(specifier, /\.html(?:\?|$)|\/raw\//, 'Private reference documents/responses must stay outside the application import graph.');
      if (specifier.includes('design-ui-v1/')) assert.match(specifier, /\/assets\//, 'Only the existing DEV artwork boundary may refer to external assets.');
    }
    const inputImport = imports.find(node => ts.isStringLiteral(node.moduleSpecifier) && node.moduleSpecifier.text === 'virtual:portico-kit-families');
    assert.ok(inputImport?.importClause?.namedBindings && ts.isNamedImports(inputImport.importClause.namedBindings));
    assert.ok(inputImport.importClause.namedBindings.elements.some(node => node.name.text === 'loadGalleryInputs'));
    const calls: ts.CallExpression[] = [];
    const visit = (node: ts.Node) => {
      if (ts.isCallExpression(node) && ts.isIdentifier(node.expression) && node.expression.text === 'loadGalleryInputs') calls.push(node);
      ts.forEachChild(node, visit);
    };
    visit(source);
    assert.equal(calls.length, 1, 'A family must not read another family or introduce a second input mechanism.');
    const call = calls[0]!;
    assert.ok(ts.isAwaitExpression(call.parent), 'Input loading must finish before the genuine model/parser consumes it.');
    assert.equal(call.arguments.length, 1);
    assert.ok(ts.isStringLiteral(call.arguments[0]!));
    assert.equal(call.arguments[0].text, family);
  });
}
