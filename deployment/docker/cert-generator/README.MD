```shell
docker build -t cert-generator .
```
```shell
docker run --rm --env-file ../.env -v /$(pwd)/output:/certs/ssl cert-generator
docker run --rm --env-file ..\.env -v ${PWD}\output:/certs/ssl cert-generator
```

