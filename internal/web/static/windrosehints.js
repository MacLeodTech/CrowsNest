// Labels, defaults, ranges and one-line help for Windrose's WorldDescription.json, shown beside each field in the
// Files tab's form. Copied from DedicatedServer.md of the current server build (0.10.0), which warns that the list and
// its ranges may change: a parameter this table does not know (a newer build adds some) still gets a field, just
// with no hint and no range check. Loads under Node too.
(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.WindroseHints = api;
})(typeof self !== "undefined" ? self : this, function () {
  // The difficulty presets for WorldPresetType. Custom is what the game switches to when WorldSettings holds values.
  const PRESETS = ["Easy", "Medium", "Hard", "Custom"];

  // name (the part after WDS.Parameter.): {label, def, help, min, max, options}. def is a JSON literal as the file
  // writes it; options (for a tag parameter) are the last words of the tag.
  const H = {
    "Coop.SharedQuests": { label: "Co-op quests", def: "true", help: "A co-op quest completed by one player completes for every player who has it active." },
    EasyExplore: { label: "Immersive exploration", def: "false", help: "Turns off the map markers for points of interest, so they are harder to find. (The file calls it EasyExplore, its old name.)" },
    MobHealthMultiplier: { label: "Enemy health", def: "1", min: 0.2, max: 5, help: "How much health enemies have." },
    MobDamageMultiplier: { label: "Enemy damage", def: "1", min: 0.2, max: 5, help: "How hard enemies hit." },
    ShipsHealthMultiplier: { label: "Enemy ship health", def: "1", min: 0.4, max: 5, help: "How much health enemy ships have." },
    ShipsDamageMultiplier: { label: "Enemy ship damage", def: "1", min: 0.2, max: 2.5, help: "How much damage enemy ships deal." },
    BoardingDifficultyMultiplier: { label: "Boarding difficulty", def: "1", min: 0.2, max: 5, help: "How many enemy sailors must be beaten to win a boarding action." },
    "Coop.StatsCorrectionModifier": { label: "Co-op enemy scaling", def: "1", min: 0, max: 2, help: "Adjusts enemy health and how fast they lose posture, based on the number of players on the server." },
    "Coop.ShipStatsCorrectionModifier": { label: "Co-op ship scaling", def: "0", min: 0, max: 2, help: "Adjusts enemy ship health based on the number of players on the server." },
    CombatDifficulty: { label: "Combat difficulty", def: '"WDS.Parameter.CombatDifficulty.Normal"', options: ["Easy", "Normal", "Hard"], help: "How hard boss encounters are and how aggressive enemies are in general." },
  };

  function get(name) {
    return Object.prototype.hasOwnProperty.call(H, name) ? H[name] : null;
  }

  return { PRESETS, get };
});
