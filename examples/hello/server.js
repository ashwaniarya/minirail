const http = require("node:http");

http
  .createServer((request, response) => {
    if (request.url === "/db") {
      response.end(process.env.DATABASE_URL ? "DATABASE_URL set" : "DATABASE_URL missing");
      return;
    }
    response.end("ok");
  })
  .listen(process.env.PORT || 8080);
