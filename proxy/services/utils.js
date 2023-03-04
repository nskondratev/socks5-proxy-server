import { dirname as stdlibDirname } from 'node:path'
import { fileURLToPath } from 'node:url'

export function dirname (fileURL) {
  return stdlibDirname(fileURLToPath(fileURL))
}
