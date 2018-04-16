try {
  process.chdir(__dirname)
} catch (err) {
  console.log('Failed to change cwd:', err)
  process.exit(1)
}

const fs = require('fs')
const path = require('path')

if (fs.existsSync(path.join(__dirname, '.env'))) {
  require('dotenv').config()
}
const container = require('./services')

const logger = container.cradle.logger.get()

logger.info(`Start proxy telegram bot. Version: ${require('./package.json').version}`)

container.cradle.telegramBot.initCommands()
