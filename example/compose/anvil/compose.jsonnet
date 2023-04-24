local anvil = import 'anvil:std';
local args = anvil.getargs();

local secrets = import 'secrets.libsonnet';

local img(image) = '%s:%s' % [image.name, image.version];

anvil.yamlMarshal({
  name: args.project,
  services: {
    postgres: {
      image: img(args.images.postgres),
      environment: {
        POSTGRES_USER: secrets.postgres.username,
        POSTGRES_PASSWORD: secrets.postgres.password,
        POSTGRES_DB: 'postgres',
        PGDATA: '/var/lib/postgresql/data/pgdata',
        POSTGRES_INITDB_ARGS: '--encoding UTF8 --locale=C --auth-local=trust --auth-host=scram-sha-256',
        POSTGRES_HOST_AUTH_METHOD: 'scram-sha-256',
        LANG: 'C',
      },
      ports: [
        {
          target: 5432,
          published: '5432',
          protocol: 'tcp',
          mode: 'host',
        },
      ],
      volumes: [
        {
          type: 'volume',
          source: 'pgdata',
          target: '/var/lib/postgresql/data',
          read_only: false,
        },
      ],
    },
    redis: {
      image: img(args.images.redis),
      entrypoint: ['redis-server'],
      command: ['/etc/redis/redis.conf'],
      ports: [
        {
          target: 6379,
          published: '6379',
          protocol: 'tcp',
          mode: 'host',
        },
      ],
      volumes: [
        {
          type: 'bind',
          source: './redisetc',
          target: '/etc/redis',
          read_only: true,
        },
      ],
    },
    minio: {
      image: img(args.images.minio),
      entrypoint: ['minio'],
      command: ['server', '/var/lib/minio/data', '--console-address', ':9001'],
      environment: {
        MINIO_ROOT_USER: secrets.minio.username,
        MINIO_ROOT_PASSWORD: secrets.minio.password,
      },
      ports: [
        {
          target: 9000,
          published: '9000',
          protocol: 'tcp',
          mode: 'host',
        },
        {
          target: 9001,
          published: '9001',
          protocol: 'tcp',
          mode: 'host',
        },
      ],
      volumes: [
        {
          type: 'volume',
          source: 'miniodata',
          target: '/var/lib/minio',
          read_only: false,
        },
      ],
    },
    nats_init: {
      image: img(args.images.anvil),
      volumes: [
        {
          type: 'bind',
          source: './natsconf',
          target: '/home/anvil/workflows',
          read_only: true,
        },
        {
          type: 'volume',
          source: 'natsconf',
          target: '/home/anvil/output',
          read_only: false,
        },
      ],
    },
    nats: {
      image: img(args.images.nats),
      entrypoint: ['nats-server'],
      command: ['--config', '/etc/nats/nats.conf'],
      ports: [
        {
          target: 4222,
          published: '4222',
          protocol: 'tcp',
          mode: 'host',
        },
      ],
      volumes: [
        {
          type: 'volume',
          source: 'natsconf',
          target: '/etc/nats',
          read_only: true,
        },
        {
          type: 'volume',
          source: 'natsdata',
          target: '/var/lib/nats',
          read_only: false,
        },
      ],
      depends_on: {
        nats_init: {
          condition: 'service_completed_successfully',
          restart: true,
        },
      },
    },
    mailhog: {
      image: img(args.images.mailhog),
      ports: [
        {
          target: 1025,
          published: '1025',
          protocol: 'tcp',
          mode: 'host',
        },
        {
          target: 8025,
          published: '8025',
          protocol: 'tcp',
          mode: 'host',
        },
      ],
    },
  },
  volumes: {
    pgdata: {},
    miniodata: {},
    natsconf: {},
    natsdata: {},
  },
})
