import { join } from 'node:path'

import log4js from 'log4js'

import { dirname } from '../services/utils.js'

const logger = log4js.getLogger()

const layout = {
  console: {
    type: 'pattern',
    pattern: '%[[%z] [%d] [%5.10p] - %]%m'
  },
  file: {
    type: 'pattern',
    pattern: '[%z] [%d] [%5.10p] - %m'
  }
}

log4js.configure({
  appenders: {
    console: { type: 'console', layout: layout.console },
    file: {
      type: 'file',
      filename: join(dirname(import.meta.url), '..', 'logs', 'cheese.log'),
      maxLogSize: 52428800,
      backups: 10,
      keepFileExt: true,
      compress: true,
      layout: layout.file
    }
  },
  categories: {
    default: { appenders: ['console', 'file'], level: process.env.LOG_LEVEL || 'info' }
  }
})

export default logger
