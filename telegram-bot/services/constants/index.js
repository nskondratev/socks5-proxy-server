const path = require('path')
const fs = require('fs')
const _ = require('lodash')

const CONSTANTS_FILES_DIR = path.join(__dirname, '..', '..', 'constants')

module.exports = container => {
  const logger = container.logger.get()
  const constants = {}
  logger.debug('Going to load constants...')
  fs
    .readdirSync(CONSTANTS_FILES_DIR)
    .filter(filename => filename.endsWith('.js'))
    .map(filename => path.join(CONSTANTS_FILES_DIR, filename))
    .forEach(fullPath => {
      constants[_.camelCase(path.basename(fullPath, path.extname(fullPath)))] = require(fullPath)
    })
  logger.debug(`Loaded constants: ${Object.keys(constants).join(', ')}`)
  return constants
}
