const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const assert = require("node:assert/strict");
const vm = require("node:vm");

const source = fs.readFileSync(path.join(__dirname, "isbn-scanner.js"), "utf8");
const context = { window: {} };
vm.runInNewContext(source, context);

const isbn = context.window.OurLibraryISBN;

test("validates ISBN checksums", () => {
  assert.equal(isbn.isValidISBN13("9780140328721"), true);
  assert.equal(isbn.isValidISBN13("9780140328722"), false);
  assert.equal(isbn.isValidISBN10("0140328726"), true);
  assert.equal(isbn.isValidISBN10("0140328727"), false);
});

test("extracts printed ISBN-13 text", () => {
  assert.equal(isbn.findValidISBN("ISBN 978-0-14-032872-1"), "9780140328721");
});

test("converts printed ISBN-10 text to ISBN-13", () => {
  assert.equal(isbn.findValidISBN("ISBN-10: 0-14-032872-6"), "9780140328721");
});

test("corrects common OCR substitutions", () => {
  assert.equal(isbn.findValidISBN("ISBN 978-O-14-O32872-1"), "9780140328721");
});

test("rejects invalid ISBN text without accepting an overlapping substring", () => {
  assert.equal(isbn.findValidISBN("ISBN 978-0-14-032872-2"), "");
});
