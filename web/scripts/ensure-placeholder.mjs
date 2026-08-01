import { writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'

await writeFile(
  resolve(import.meta.dirname, '../../internal/webui/dist/placeholder.txt'),
  'Run `npm run build` in `web/` before building a release binary.\n',
)

