import fs from 'node:fs'
import path from 'node:path'

import dotenv from 'dotenv'

import logger from './services/logger.js'
import { client as redis, Store } from './services/redis.js'
import { Bot } from './services/bot.js'
import { dirname } from './services/utils.js'

if (fs.existsSync(path.join(dirname(import.meta.url), '.env'))) {
  dotenv.config()
}

const store = new Store(redis)

const bot = new Bot(store, logger)
bot.run()

logger.info('Start proxy telegram bot')
