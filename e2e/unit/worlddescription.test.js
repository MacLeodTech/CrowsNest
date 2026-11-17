// Unit tests for the Files tab's Windrose WorldDescription.json reader: `node --test e2e/unit/*.test.js`.
const test = require("node:test");
const assert = require("node:assert/strict");
const WD = require("../../internal/web/static/worlddescription.js");
const Hints = require("../../internal/web/static/windrosehints.js");

// The layout of a real 0.10.0 server's file: tabs, braces on their own lines, escaped tag keys, a big CreationTime.
const FILE = `{
	"Version": 1,
	"WorldDescription":
	{
		"islandId": "320FFB65E5004723E680079D58662001",
		"WorldName": "",
		"CreationTime": 6.3913260023461005e+17,
		"WorldPresetType": "Medium",
		"WorldSettings":
		{
			"BoolParameters":
			{
				"{\\"TagName\\": \\"WDS.Parameter.Coop.SharedQuests\\"}": true,
				"{\\"TagName\\": \\"WDS.Parameter.EasyExplore\\"}": false
			},
			"FloatParameters":
			{
				"{\\"TagName\\": \\"WDS.Parameter.MobHealthMultiplier\\"}": 1,
				"{\\"TagName\\": \\"WDS.Parameter.ShipsDamageMultiplier\\"}": 1.5,
				"{\\"TagName\\": \\"WDS.Parameter.Coop.ShipStatsCorrectionModifier\\"}": 0
			},
			"TagParameters":
			{
				"{\\"TagName\\": \\"WDS.Parameter.CombatDifficulty\\"}":
				{
					"TagName": "WDS.Parameter.CombatDifficulty.Normal"
				}
			}
		}
	}
}`;

const byName = (m, name) => m.items.find((i) => i.name === name);

test("parse lists the preset and every parameter with the value as written", () => {
  const m = WD.parse(FILE);
  assert.deepEqual(m.items.map((i) => `${i.kind}:${i.name}=${i.value}`), [
    'preset:WorldPresetType="Medium"',
    "bool:Coop.SharedQuests=true",
    "bool:EasyExplore=false",
    "number:MobHealthMultiplier=1",
    "number:ShipsDamageMultiplier=1.5",
    "number:Coop.ShipStatsCorrectionModifier=0",
    'tag:CombatDifficulty="WDS.Parameter.CombatDifficulty.Normal"',
  ]);
});

test("set changes only the one value: every other byte, including CreationTime, stays", () => {
  const m = WD.parse(FILE);
  let t = WD.set(FILE, byName(m, "MobHealthMultiplier"), "2.5");
  assert.equal(t, FILE.replace('MobHealthMultiplier\\"}": 1,', 'MobHealthMultiplier\\"}": 2.5,'));
  assert.ok(t.includes("6.3913260023461005e+17"));
  JSON.parse(t);
  // positions are stale after an edit, so a caller parses again
  const m2 = WD.parse(t);
  t = WD.set(t, byName(m2, "CombatDifficulty"), WD.quote("WDS.Parameter.CombatDifficulty.Hard"));
  t = WD.set(t, byName(WD.parse(t), "EasyExplore"), "true");
  t = WD.set(t, byName(WD.parse(t), "WorldPresetType"), '"Custom"');
  const doc = JSON.parse(t).WorldDescription;
  assert.equal(doc.WorldPresetType, "Custom");
  assert.equal(doc.WorldSettings.TagParameters['{"TagName": "WDS.Parameter.CombatDifficulty"}'].TagName, "WDS.Parameter.CombatDifficulty.Hard");
  assert.equal(doc.WorldSettings.BoolParameters['{"TagName": "WDS.Parameter.EasyExplore"}'], true);
  assert.equal(doc.WorldSettings.FloatParameters['{"TagName": "WDS.Parameter.MobHealthMultiplier"}'], 2.5);
});

test("a parameter it does not know still gets an item, and one it cannot read is left alone", () => {
  const extra = FILE.replace('"{\\"TagName\\": \\"WDS.Parameter.EasyExplore\\"}": false',
    '"{\\"TagName\\": \\"WDS.Parameter.BrandNew.Thing\\"}": true,\n\t\t\t\t"not json": false,\n\t\t\t\t"{\\"TagName\\": \\"Other.Thing\\"}": false');
  const m = WD.parse(extra);
  assert.ok(byName(m, "BrandNew.Thing"));
  assert.equal(m.items.filter((i) => i.kind === "bool").length, 2); // SharedQuests + BrandNew.Thing
});

test("anything that is not a world file is not read", () => {
  assert.equal(WD.parse("not json"), null);
  assert.equal(WD.parse("{}"), null);
  assert.equal(WD.parse('{"WorldDescription": {}}'), null);
  assert.equal(WD.parse('{"Version": 1, "ServerDescription_Persistent": {"ServerName": "x"}}'), null);
});

test("number helpers", () => {
  assert.ok(WD.isNumber("1.5") && WD.isNumber("-2") && WD.isNumber("3e2") && WD.isNumber("0.2"));
  assert.ok(!WD.isNumber("") && !WD.isNumber("abc") && !WD.isNumber("1.2.3") && !WD.isNumber("Infinity") && !WD.isNumber(".5") && !WD.isNumber("5.") && !WD.isNumber("01"));
  assert.ok(WD.sameValue("1", "1.0") && !WD.sameValue("1", "2"));
  assert.equal(WD.quote('a"b'), '"a\\"b"');
  assert.equal(WD.label("Coop.StatsCorrectionModifier"), "Coop Stats Correction Modifier");
});

test("hints cover the documented parameters and know nothing of an unknown one", () => {
  for (const n of ["Coop.SharedQuests", "EasyExplore", "MobHealthMultiplier", "MobDamageMultiplier", "ShipsHealthMultiplier",
    "ShipsDamageMultiplier", "BoardingDifficultyMultiplier", "Coop.StatsCorrectionModifier", "Coop.ShipStatsCorrectionModifier", "CombatDifficulty"]) {
    const h = Hints.get(n);
    assert.ok(h && h.label && h.help && h.def !== undefined, n);
  }
  assert.equal(Hints.get("Nope"), null);
  assert.equal(Hints.get("constructor"), null);
  assert.deepEqual(Hints.PRESETS, ["Easy", "Medium", "Hard", "Custom"]);
});
