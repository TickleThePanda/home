const { spawnSync } = require("node:child_process");
const { join } = require("node:path");

// Cleanup only: never fail the job from here.
spawnSync("bash", [join(__dirname, "disconnect.sh")], { stdio: "inherit" });
