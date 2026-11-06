---
name: template-check
description: "Valida un stack de plantillas/ generando un proyecto de prueba con el binario actual: bash -n de los scripts, make help y make -n, docker compose config con overlays (si hay Docker) y zone-check --owners. Úsala después de cambiar una plantilla; la ejecuta `make template-check STACK=<id>`."
---
# template-check

```bash
make template-check STACK=nextjs     # nextjs | fastapi | laravel | spring
```

Genera el proyecto en un directorio temporal con `--db postgres --redis` (los mismos overlays en todos los stacks), imprime `OK`/`FAIL`/`SKIP` por comprobación y lo borra al terminar. Sale con 0 si todo pasó.

No construye imágenes ni levanta contenedores: para eso está `make e2e STACK=<id>` (pruebas 12 y de producción, con Docker).
