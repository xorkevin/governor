local secrets = import 'secrets.libsonnet';

{
  username: secrets.nats.username,
  password: secrets.nats.password,
}
