// Unit tests for the Files tab's OptionSettings reader: `node --test e2e/unit/*.test.js` (no browser needed).
const test = require("node:test");
const assert = require("node:assert/strict");
const OS = require("../../internal/web/static/optionsettings.js");

const HEAD = "[/Script/Pal.PalGameWorldSettings]\n";
const LINE =
  'OptionSettings=(Difficulty=None,ExpRate=1.000000,bIsPvP=False,ServerName="My, Server (EU)",' +
  'AdminPassword="s3cret",CrossplayPlatforms=(Steam,Xbox,PS5,Mac),Empty="")';
const FILE = HEAD + LINE + "\n";

test("parse reads every setting, keeping quoted commas and nested lists whole", () => {
  const m = OS.parse(FILE);
  assert.deepEqual(m.items.map((i) => i.key), ["Difficulty", "ExpRate", "bIsPvP", "ServerName", "AdminPassword", "CrossplayPlatforms", "Empty"]);
  assert.equal(m.items[3].value, '"My, Server (EU)"');
  assert.equal(m.items[5].value, "(Steam,Xbox,PS5,Mac)");
  assert.equal(m.before, HEAD + "OptionSettings=");
  assert.equal(m.after, "\n");
});

test("expand puts each setting on its own line and collapse folds it back to the exact original", () => {
  const x = OS.expand(FILE);
  assert.ok(x.split("\n").length > 8, x);
  assert.match(x, /^OptionSettings=\($/m);
  assert.match(x, /^    ServerName="My, Server \(EU\)",$/m);
  assert.equal(OS.collapse(x), FILE);
  assert.equal(OS.collapse(OS.expand(FILE)), FILE);
});

test("an edit in the expanded text collapses into the one-line form", () => {
  const edited = OS.expand(FILE).replace("ExpRate=1.000000", "ExpRate=2.500000");
  assert.equal(OS.collapse(edited), FILE.replace("ExpRate=1.000000", "ExpRate=2.500000"));
});

test("text it does not fully understand is returned untouched", () => {
  for (const t of ["", "[Section]\nA=1\n", "OptionSettings=(A=1,B", "OptionSettings=(A=1,,B=2)", "OptionSettings=(A=1,notakeyvalue)", "OptionSettings=(1bad=2)"]) {
    assert.equal(OS.parse(t), null, JSON.stringify(t));
    assert.equal(OS.expand(t), t);
    assert.equal(OS.collapse(t), t);
  }
});

test("an empty list and lines around the setting survive", () => {
  assert.deepEqual(OS.parse("OptionSettings=()").items, []);
  assert.equal(OS.collapse(OS.expand("OptionSettings=()")), "OptionSettings=()");
  const f = "; comment\n[S]\nOptionSettings=(A=1,B=2)\nOther=(X=Y)\n";
  assert.equal(OS.collapse(OS.expand(f)), f);
  assert.match(OS.expand(f), /^Other=\(X=Y\)$/m);
});

test("kind and label", () => {
  assert.equal(OS.kind("True"), "bool");
  assert.equal(OS.kind("False"), "bool");
  assert.equal(OS.kind("true"), "bool");
  assert.equal(OS.boolText("True", false), "False");
  assert.equal(OS.boolText("false", true), "true");
  assert.equal(OS.kind("1.000000"), "number");
  assert.equal(OS.kind("-3"), "number");
  assert.equal(OS.kind('"x"'), "string");
  assert.equal(OS.kind('""'), "string");
  assert.equal(OS.kind("None"), "other");
  assert.equal(OS.kind("(A,B)"), "other");
  assert.equal(OS.label("DayTimeSpeedRate"), "Day Time Speed Rate");
  assert.equal(OS.label("bIsPvP"), "Is PvP");
  assert.equal(OS.label("PalEggDefaultHatchingTime"), "Pal Egg Default Hatching Time");
  assert.equal(OS.label("RESTAPIPort"), "RESTAPI Port");
});

test("sameValue compares numbers as numbers and booleans without case", () => {
  assert.ok(OS.sameValue("1.0", "1.000000"));
  assert.ok(OS.sameValue("true", "True"));
  assert.ok(!OS.sameValue("2", "1.000000"));
  assert.ok(!OS.sameValue('"a"', '"A"'));
  assert.ok(!OS.sameValue("True", "1"));
  assert.ok(OS.sameValue("None", "None"));
});

test("Palworld hints: known keys carry a default and help, unknown keys none, no passwords", () => {
  const PH = require("../../internal/web/static/palworldhints.js");
  const h = PH.get("ExpRate");
  assert.equal(h.def, "1.000000");
  assert.ok(h.help.length > 5);
  assert.equal(PH.get("NoSuchSetting"), null);
  assert.equal(PH.get("toString"), null);
  assert.equal(PH.get("AdminPassword"), null);
  assert.equal(PH.get("ServerPassword"), null);
  // A default must read as the same kind of value as the form would show for it.
  for (const k of ["bIsPvP", "DayTimeSpeedRate", "DropItemMaxNum", "ServerName", "DeathPenalty", "CrossplayPlatforms"]) {
    assert.ok(PH.get(k).def !== undefined, k);
  }
  assert.equal(OS.kind(PH.get("bIsPvP").def), "bool");
  assert.equal(OS.kind(PH.get("ServerName").def), "string");
});
