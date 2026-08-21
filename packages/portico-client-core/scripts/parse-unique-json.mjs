/** Parse JSON while rejecting duplicate object keys at every nesting level. */
export function parseUniqueJSON(source, label = "JSON document") {
  if (typeof source !== "string") throw new TypeError(`${label} must be text.`);
  let offset = 0;

  const fail = (message) => {
    throw new SyntaxError(`${label}: ${message} at offset ${offset}.`);
  };
  const isWhitespace = (character) => character === " " || character === "\t" || character === "\r" || character === "\n";
  const isValueDelimiter = (character) => character === "," || character === "}" || character === "]";
  const whitespace = () => {
    while (offset < source.length && isWhitespace(source[offset])) offset += 1;
  };
  const string = () => {
    if (source[offset] !== '"') fail("expected a string");
    const start = offset;
    offset += 1;
    while (offset < source.length) {
      const character = source[offset++];
      if (character === '"') {
        try {
          return JSON.parse(source.slice(start, offset));
        } catch {
          fail("invalid string");
        }
      }
      if (character === "\\") {
        if (offset >= source.length) fail("unterminated escape");
        if (source[offset] === "u") {
          if (!/^[0-9a-fA-F]{4}$/.test(source.slice(offset + 1, offset + 5))) fail("invalid Unicode escape");
          offset += 5;
        } else {
          if (!/["\\/bfnrt]/.test(source[offset])) fail("invalid escape");
          offset += 1;
        }
      } else if (character.charCodeAt(0) < 0x20) {
        fail("unescaped control character");
      }
    }
    fail("unterminated string");
  };
  const value = (path) => {
    whitespace();
    if (source[offset] === "{") {
      offset += 1;
      const result = {};
      const keys = new Set();
      whitespace();
      if (source[offset] === "}") {
        offset += 1;
        return result;
      }
      while (offset < source.length) {
        whitespace();
        const key = string();
        if (keys.has(key)) fail(`duplicate key ${JSON.stringify(key)} in ${path}`);
        keys.add(key);
        whitespace();
        if (source[offset++] !== ":") fail("expected ':' after object key");
        result[key] = value(`${path}.${key}`);
        whitespace();
        const delimiter = source[offset++];
        if (delimiter === "}") return result;
        if (delimiter !== ",") fail("expected ',' or '}'");
      }
      fail("unterminated object");
    }
    if (source[offset] === "[") {
      offset += 1;
      const result = [];
      whitespace();
      if (source[offset] === "]") {
        offset += 1;
        return result;
      }
      while (offset < source.length) {
        result.push(value(`${path}[${result.length}]`));
        whitespace();
        const delimiter = source[offset++];
        if (delimiter === "]") return result;
        if (delimiter !== ",") fail("expected ',' or ']'");
      }
      fail("unterminated array");
    }
    if (source[offset] === '"') return string();
    const start = offset;
    while (offset < source.length && !isWhitespace(source[offset]) && !isValueDelimiter(source[offset])) offset += 1;
    if (start === offset) fail("expected a value");
    try {
      return JSON.parse(source.slice(start, offset));
    } catch {
      fail("invalid value");
    }
  };

  const result = value("$");
  whitespace();
  if (offset !== source.length) fail("unexpected trailing content");
  return result;
}
