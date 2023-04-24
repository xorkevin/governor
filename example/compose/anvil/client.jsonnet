local anvil = import 'anvil:std';
local args = anvil.getargs();

{
  http: {
    baseurl: '%s/api' % args.server.baseurl,
    timeout: '5s',
  },
  token: {
    issuer: args.server.baseurl,
    audience: args.server.tokenaudience,
  },
}
