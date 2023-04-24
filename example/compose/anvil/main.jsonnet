local anvil = import 'anvil:std';

local secrets = import 'secrets.libsonnet';

local images = {
  governor: {
    name: 'docker.pkg.dev.localhost:8080/governor',
    version: 'latest',
  },
  anvil: {
    name: 'xorkevin/anvil',
    version: 'latest',
  },
  postgres: {
    name: 'postgres',
    version: '15.2-alpine3.17',
  },
  redis: {
    name: 'redis',
    version: '7.0.11-alpine3.17',
  },
  minio: {
    name: 'minio/minio',
    version: 'RELEASE.2023-04-20T17-56-55Z',
  },
  nats: {
    name: 'nats',
    version: '2.9.16-alpine3.17',
  },
  mailhog: {
    name: 'mailhog/mailhog',
    version: 'v1.0.1',
  },
};

local outputdir = 'governor';

local server = {
  hostname: 'governor.dev.localhost',
  port: '8080',
  host: '%s:%s' % [self.hostname, self.port],
  shortlinkhost: 'go.%s' % self.host,
  protocol: 'http',
  baseurl: '%s://%s' % [self.protocol, self.host],
  uibaseurl: 'http://localhost:3000',
  shortlinkbaseurl: '%s://%s' % [self.protocol, self.shortlinkhost],
  httprealm: 'governor',
  maildomain: 'xorkevin.com',
  mailspfdomain: 'mail.xorkevin.com',
  mailinglistdomain: 'lists.%s' % self.hostname,
  orgmailinglistdomain: 'org.%s' % self.mailinglistdomain,
  tokenaudience: 'governor',
};

{
  version: 'xorkevin.dev/anvil/v1alpha1',
  templates: [
    {
      kind: 'jsonnetstr',
      path: 'compose.jsonnet',
      args: {
        project: 'governor',
        images: images,
      },
      output: '%s/compose.yaml' % outputdir,
    },
    {
      kind: 'gotmpl',
      path: 'redis.conf.tmpl',
      args: {
        username: secrets.redis.username,
        passhash: anvil.sha256hex(secrets.redis.password),
      },
      output: '%s/redisetc/redis.conf' % outputdir,
    },
    {
      kind: 'staticfile',
      path: 'nats.conf.tmpl',
      output: '%s/natsconf/nats.conf.tmpl' % outputdir,
    },
    {
      kind: 'jsonnet',
      path: 'natsauth.jsonnet',
      output: '%s/natsconf/natsauth.json' % outputdir,
    },
    {
      kind: 'staticfile',
      path: 'nats.star',
      output: '%s/natsconf/main.star' % outputdir,
    },
    {
      kind: 'jsonnet',
      path: 'governor.jsonnet',
      args: {
        outputdir: anvil.pathJoin(['compose', outputdir]),
        server: server,
      },
      output: '%s/governor.json' % outputdir,
    },
    {
      kind: 'jsonnet',
      path: 'secretsfile.jsonnet',
      output: '%s/secrets.json' % outputdir,
    },
    {
      kind: 'jsonnet',
      path: 'mockdns.jsonnet',
      output: '%s/mockdns.json' % outputdir,
    },
    {
      kind: 'jsonnet',
      path: 'client.jsonnet',
      args: {
        server: server,
      },
      output: '%s/client.json' % outputdir,
    },
  ],
}
