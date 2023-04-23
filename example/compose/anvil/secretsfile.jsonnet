local tokenextkey = importstr 'tokenextkey.pem';
local syspubkey = importstr 'syspubkey.pem';

local secrets = import 'secrets.libsonnet';

{
  data: {
    setupsecret: {
      secret: 'admin',
    },
    dbauth: secrets.postgres,
    kvauth: secrets.redis,
    objauth: secrets.minio,
    pubsubauth: secrets.nats,
    eventsauth: secrets.nats,
    mailauth: {
      username: 'admin',
      password: 'admin',
    },
    tokensecret: {
      secrets: [
        '$hs512$secret',
      ],
      extkeys: [
        tokenextkey,
      ],
      syspubkeys: [
        syspubkey,
      ],
    },
    otpkey: {
      secrets: [
        '$xc20p$XudfGvCqdVdpmTCXlJ_QmauZZLyMS2kWtecv0HEoOhQ',
      ],
    },
    mailkey: {
      secrets: [
        '$xc20p$XudfGvCqdVdpmTCXlJ_QmauZZLyMS2kWtecv0HEoOhQ',
      ],
    },
  },
}
