// Windrose's WorldDescription.json: the difficulty preset and the world parameters, which sit under
// WorldDescription.WorldSettings as BoolParameters, FloatParameters and TagParameters, each keyed by a string that
// itself holds JSON ("{\"TagName\": \"WDS.Parameter.MobHealthMultiplier\"}"). This module finds where each value
// sits in the text and replaces just that value, so a save changes only what you changed: the file's own layout,
// key order and number formatting stay exactly as they were (a parse and re-print would have reformatted it and
// rounded its big CreationTime). It returns null for anything it does not fully understand, so a file it cannot
// read is shown as plain text and never rewritten. Used by the Files tab (app.js); also loads under Node for its
// unit test.
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.WorldDescription = api;
})(typeof self !== "undefined" ? self : this, function () {
  const PREFIX = "WDS.Parameter.";

  // A tiny JSON scanner that keeps where every value starts and ends. Input has already passed JSON.parse, so it
  // only has to find the spans, not judge the syntax.
  function scan(text) {
    let i = 0;
    const ws = () => { while (i < text.length && /\s/.test(text[i])) i++; };
    function str() {
      const start = i;
      i++; // opening quote
      while (text[i] !== '"') i += text[i] === "\\" ? 2 : 1;
      i++;
      return { type: "string", start, end: i, value: JSON.parse(text.slice(start, i)) };
    }
    function value() {
      ws();
      const c = text[i];
      if (c === "{") {
        const node = { type: "object", start: i, props: [] };
        i++; ws();
        if (text[i] === "}") { i++; node.end = i; return node; }
        for (;;) {
          ws();
          const key = str();
          ws(); i++; // the colon
          const v = value();
          node.props.push({ key: key.value, value: v });
          ws();
          if (text[i] === ",") { i++; continue; }
          i++; // the closing brace
          node.end = i;
          return node;
        }
      }
      if (c === "[") {
        const node = { type: "array", start: i };
        i++; ws();
        if (text[i] === "]") { i++; node.end = i; return node; }
        for (;;) {
          value(); ws();
          if (text[i] === ",") { i++; continue; }
          i++;
          node.end = i;
          return node;
        }
      }
      if (c === '"') return str();
      const m = /^(true|false|null|-?\d+(\.\d+)?([eE][+-]?\d+)?)/.exec(text.slice(i));
      const node = { type: /^(true|false)$/.test(m[0]) ? "bool" : m[0] === "null" ? "null" : "number", start: i, end: i + m[0].length, value: m[0] };
      i = node.end;
      return node;
    }
    return value();
  }

  const prop = (node, key) => (node && node.type === "object" ? (node.props.find((p) => p.key === key) || {}).value : undefined);

  // The suffix of a parameter key ("MobHealthMultiplier"), or null when the key is not the usual tag JSON.
  function tagName(key) {
    try {
      const t = JSON.parse(key).TagName;
      return typeof t === "string" && t.startsWith(PREFIX) ? t.slice(PREFIX.length) : null;
    } catch (e) { return null; }
  }

  // parse returns {items} or null. Each item is {id, section, name, kind, value, start, end}: kind is "preset",
  // "bool", "number" or "tag"; value is the literal as written in the file (true, 1.5, "WDS...Normal"); start and
  // end are its place in the text. An entry it does not recognise is simply not listed (and stays in the file).
  function parse(text) {
    let doc;
    try { JSON.parse(text); doc = scan(text); } catch (e) { return null; }
    const wd = prop(doc, "WorldDescription");
    if (!wd || wd.type !== "object") return null;
    const items = [];
    const preset = prop(wd, "WorldPresetType");
    if (preset && preset.type === "string") {
      items.push({ id: "preset", section: "", name: "WorldPresetType", kind: "preset", value: text.slice(preset.start, preset.end), start: preset.start, end: preset.end });
    }
    const settings = prop(wd, "WorldSettings");
    for (const [section, kind] of [["BoolParameters", "bool"], ["FloatParameters", "number"], ["TagParameters", "tag"]]) {
      const group = prop(settings, section);
      if (!group || group.type !== "object") continue;
      for (const p of group.props) {
        const name = tagName(p.key);
        if (name === null) continue;
        let node = p.value;
        if (kind === "tag") node = prop(node, "TagName");
        if (!node || node.type !== (kind === "tag" ? "string" : kind)) continue;
        items.push({ id: `${section}:${name}`, section, name, kind, value: text.slice(node.start, node.end), start: node.start, end: node.end });
      }
    }
    return items.length ? { items } : null;
  }

  // set returns the text with item's value replaced by literal (already in JSON form), nothing else touched.
  function set(text, item, literal) {
    return text.slice(0, item.start) + literal + text.slice(item.end);
  }

  // JSON's own number syntax (no ".5", no "5."), so a value it accepts can never make the file invalid.
  const isNumber = (s) => /^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$/.test(s) && Number.isFinite(Number(s));

  // The JSON literal for a text value: a quoted string, with quotes and backslashes escaped.
  const quote = (s) => JSON.stringify(s);

  // True when two literals mean the same thing: numbers compare as numbers, anything else must match exactly.
  function sameValue(a, b) {
    if (a === b) return true;
    return isNumber(a) && isNumber(b) && Number(a) === Number(b);
  }

  // "Coop.StatsCorrectionModifier" -> "Coop Stats Correction Modifier", for a parameter the hints do not know.
  function label(name) {
    return name.replace(/\./g, " ").replace(/([a-z0-9])([A-Z])/g, "$1 $2");
  }

  return { PREFIX, parse, set, isNumber, quote, sameValue, label };
});
