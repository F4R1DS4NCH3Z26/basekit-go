# Contrato: <feature>
Estado: borrador | cerrado
Referencia: docs/guias/02_especificacion_cli.md §<n> (y la sección de la guía que aplique)

## Común
- Comportamiento: <qué debe pasar, con los textos exactos que pidió el usuario>
- Fuera de alcance: <qué no se toca>
- Verificación: `make verify` (+ `make template-check STACK=<id>` / `make e2e STACK=<id>` si aplica)

## core-writer
- <firmas y comportamiento en internal/fsx, stack, manifest o usage, o "(ninguno)">

## adapters-writer
- <adaptadores o procesos externos, o "(ninguno)">

## commands-writer
- <flags, preguntas, salida y códigos de salida, o "(ninguno)">

## templates-writer
- <plantillas que cambian, o "(ninguno)">

## qa-writer
- <un caso por regla: entrada → salida esperada, código de salida y archivos>
