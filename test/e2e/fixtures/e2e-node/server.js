// Proves the node driver: dependencies install step, privilege drop and routing.
require("http")
  .createServer((req, res) => res.end(`E2E-NODE-OK uid=${process.getuid()}\n`))
  .listen(process.env.PORT, process.env.HOST);
