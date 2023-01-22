package governor

//func TestVaultSecretReader(t *testing.T) {
//	t.Parallel()
//
//	tabReplacer := strings.NewReplacer("\t", "  ")
//
//	assert := require.New(t)
//
//	// prevent github.com/hashicorp/vault/sdk log at /physical/error.go
//	// "creating error injector"
//	hclog.DefaultOutput = io.Discard
//
//	vaultcore, _, rootToken := vault.TestCoreUnsealedWithConfig(t, &vault.CoreConfig{
//		Logger:          hclog.NewNullLogger(),
//		BuiltinRegistry: vault.NewMockBuiltinRegistry(),
//		LogicalBackends: map[string]vaultlogical.Factory{
//			"kv": kv.Factory,
//		},
//	})
//	vaultserver := httptest.NewServer(vaulthttp.Handler(&vault.HandlerProperties{
//		Core: vaultcore,
//		ListenerConfig: &vaultconfigutil.Listener{
//			Address: "localhost",
//		},
//	}))
//	t.Cleanup(func() {
//		vaultserver.Close()
//	})
//
//	vconfig := vaultapi.DefaultConfig()
//	assert.NoError(vconfig.Error)
//	vconfig.Address = vaultserver.URL
//	vaultclient, err := vaultapi.NewClient(vconfig)
//	assert.NoError(err)
//	vaultclient.SetToken(rootToken)
//
//	assert.NoError(vaultclient.Sys().Mount("kv", &vaultapi.MountInput{
//		Type: "kv",
//		Options: map[string]string{
//			"version": "2",
//		},
//	}))
//
//	_, err = vaultclient.KVv2("kv").Put(context.Background(), "govtest/testsecret", map[string]interface{}{
//		"secret": "secretval",
//	})
//	assert.NoError(err)
//
//	var logbuf bytes.Buffer
//
//	config := newSettings(Opts{
//		Appname: "govtest",
//		Version: Version{
//			Num:  "test",
//			Hash: "dev",
//		},
//		Description:  "test gov server",
//		EnvPrefix:    "gov",
//		ClientPrefix: "govc",
//		ConfigReader: strings.NewReader(tabReplacer.Replace(`
//http:
//	addr: ':8080'
//	basepath: /api
//vault:
//	addr: ` + vaultserver.URL + `
//	token: ` + rootToken + `
//testservice:
//	asecret: kv/data/govtest/testsecret
//`)),
//		LogWriter: &logbuf,
//	})
//
//	assert.NoError(config.init(context.Background(), Flags{}))
//	assert.Equal("vault source; addr: "+vaultserver.URL, config.vault.Info())
//
//	reader := config.reader(serviceOpt{
//		name: "testservice",
//		url:  "/null/test",
//	})
//
//	var testsecret struct {
//		Secret string `mapstructure:"secret"`
//	}
//	assert.NoError(reader.GetSecret(context.Background(), "asecret", 60, &testsecret))
//
//	assert.Equal("secretval", testsecret.Secret)
//}
