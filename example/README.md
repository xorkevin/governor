# Example governor project

```
NAME
        governor - example project

PROLOG
        help             Print this help
        all              Default
            Depends on build

DEV ENV
        devgen           Generate k8s resources
        devsecrets       Register vault resources
        devregister      Register k8s resources
        devunregister    Unregister k8s resources
        devinit          Init resources
            Depends on devgen devsecrets devregister
        devconfig        Set app config
        devup            Deploy k8s resources to cluster
        devdown          Destroy k8s resources
        devnsdown        Destroy k8s namespace

DOCKER COMPOSE DEV ENV
        dc-devgen        Generate docker compose resources
        dc-devup         Deploy docker-compose resources
        dc-devdown       Destroy docker-compose resources

APP BUILD
        clean            Remove build artifacts
        build            Build app

DOCKER
        build-docker     Build docker image
            Depends on build
        publish-docker   Publish docker image
        docker           Release new image
            Depends on build-docker publish-docker

APP DEPLOY
        skaffoldinit     Initialize skaffold
        dev              Deploy app to cluster
            Depends on build
        devstop          Stop running app
        tail             Tail log lines
        setupfirst       Run app first setup
            Depends on build
        setup            Run app setup
            Depends on build

LOCAL DEV
        run              Runs app locally
            Depends on build
        runsetup         Runs setup locally
            Depends on build
```
