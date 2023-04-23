local dkimtxt = importstr 'mockdnsdkim.txt';

{
  data: {
    'mail.governor.dev.localhost.': {
      TXT: [
        'v=spf1 ip4:127.0.0.1 ip6:::1 ~all',
      ],
    },
    'tests._domainkey.governor.dev.localhost.': {
      TXT: dkimtxt,
    },
  },
}
