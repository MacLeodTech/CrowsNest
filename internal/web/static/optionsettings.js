// Unreal-style "OptionSettings=(A=1,B=\"x\",C=(P,Q))" lines, as in Palworld's PalWorldSettings.ini: one very long
// line holding every setting. This module reads that line into a list, writes it back one setting per line (for
// reading and editing) and folds it into the single line the game expects (for saving). It touches nothing else in
// the file, and returns null / the text unchanged for anything it does not fully understand, so a file it cannot
// read is never rewritten. Used by the Files tab (app.js); also loads under Node for its unit test.
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.OptionSettings = api;
})(typeof self !== "undefined" ? self : this, function () {
  const START = /^([ \t]*OptionSettings[ \t]*=[ \t]*)\(/m;

  // The index of the ")" that closes the "(" at text[open], skipping quoted strings and nested brackets; -1 if
  // the brackets never balance.
  function matchParen(text, open) {
    let depth = 0, quoted = false;
    for (let i = open; i < text.length; i++) {
      const c = text[i];
      if (c === '"') quoted = !quoted;
      else if (quoted) continue;
      else if (c === "(") depth++;
      else if (c === ")" && --depth === 0) return i;
    }
    return -1;
  }

  // Split on the commas that are not inside quotes or nested brackets.
  function splitTop(inner) {
    const parts = [];
    let depth = 0, quoted = false, from = 0;
    for (let i = 0; i < inner.length; i++) {
      const c = inner[i];
      if (c === '"') quoted = !quoted;
      else if (quoted) continue;
      else if (c === "(") depth++;
      else if (c === ")") depth--;
      else if (c === "," && depth === 0) { parts.push(inner.slice(from, i)); from = i + 1; }
    }
    parts.push(inner.slice(from));
    return parts;
  }

  // parse returns {before, items: [{key, value}], after}, or null when the text has no OptionSettings=(...) or it
  // is not a plain Key=Value list.
  function parse(text) {
    const m = START.exec(text);
    if (!m) return null;
    const open = m.index + m[0].length - 1;
    const close = matchParen(text, open);
    if (close < 0) return null;
    const inner = text.slice(open + 1, close).trim();
    const items = [];
    if (inner !== "") {
      for (const part of splitTop(inner)) {
        const p = part.trim();
        const eq = p.indexOf("=");
        if (eq < 1) return null;
        const key = p.slice(0, eq).trim();
        if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) return null;
        items.push({ key, value: p.slice(eq + 1).trim() });
      }
    }
    return { before: text.slice(0, open), items, after: text.slice(close + 1) };
  }

  // Both writers produce text that parse() reads back to the same items.
  function expandModel(model) {
    if (model.items.length === 0) return model.before + "()" + model.after;
    const lines = model.items.map((i) => `    ${i.key}=${i.value}`);
    return model.before + "(\n" + lines.join(",\n") + "\n)" + model.after;
  }

  function collapseModel(model) {
    return model.before + "(" + model.items.map((i) => `${i.key}=${i.value}`).join(",") + ")" + model.after;
  }

  function expand(text) { const m = parse(text); return m ? expandModel(m) : text; }
  function collapse(text) { const m = parse(text); return m ? collapseModel(m) : text; }

  // What kind of control a value wants: True/False, a number, a quoted string, or anything else (an enum word, a
  // bracketed list) edited as plain text.
  function kind(value) {
    if (/^(true|false)$/i.test(value)) return "bool";
    if (/^[+-]?(\d+\.?\d*|\.\d+)$/.test(value)) return "number";
    if (/^"[^"]*"$/.test(value)) return "string";
    return "other";
  }

  // The text for a switch turned on or off, in the same style as the value it replaces (Palworld writes True
  // and False for most settings but lowercase for a few).
  function boolText(original, on) {
    if (original === original.toLowerCase()) return on ? "true" : "false";
    return on ? "True" : "False";
  }

  // "bIsPvP" -> "Is PvP", "DayTimeSpeedRate" -> "Day Time Speed Rate", for the form's labels.
  function label(key) {
    let k = key.replace(/^b(?=[A-Z])/, "");
    k = k.replace(/([a-z0-9])([A-Z][a-z]|[A-Z]{2,})/g, "$1 $2").replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2");
    return k.replace(/_/g, " ");
  }

  return { parse, expand, collapse, expandModel, collapseModel, kind, label, boolText };
});
