---
name: cli-verify
description: Verificación silenciosa del CLI en Go (gofmt, go vet, staticcheck si está instalado y go test) con salida mínima. Úsala antes de dar por terminado cualquier cambio de código; la ejecuta `make verify`.
---
# cli-verify

```bash
make verify                          # todo el módulo
make verify PKG=./internal/fsx/...   # un paquete
```

Imprime `OK`/`FAIL` por paso y, si algo falla, solo las líneas relevantes (máximo `MAX_LINES`, 30 por defecto). Sale con 0 si todo pasó. No modifica `go.mod` ni `go.sum` (`GOFLAGS=-mod=readonly`); si falta algún módulo en la caché, `go` lo descarga.

Reporta el resultado en el campo `VERIFY` de tu respuesta.
