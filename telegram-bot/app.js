import fs from 'node:fs'
import path from 'node:path'

import dotenv from 'dotenv'

import packageJSON from './package.json' assert { type: 'json' }
import { dirname } from './services/utils.js'

if (fs.existsSync(path.join(dirname(import.meta.url), '.env'))) {
  dotenv.config()
}
const container = require('./services')

const logger = container.cradle.logger.get()

container.cradle.redis.connect()
  .then(() => {
    logger.info(`Start proxy telegram bot. Version: ${packageJSON.version}`)

    container.cradle.telegramBot.initCommands()
  })
  .catch(err => {
    logger.error(`Failed to connect to Redis: ${err}`)
  })
