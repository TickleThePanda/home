const { spawnSync } = require("node:child_process");
const { join } = require("node:path");

const result = spawnSync("bash", [join(__dirname, "connect.sh")], { stdio: "inherit" });
process.exit(result.status ?? 1);
