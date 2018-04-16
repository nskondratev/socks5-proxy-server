const TelegramBot = require('node-telegram-bot-api')
const path = require('path')
const fs = require('fs')

const COMMANDS_DIR = path.join(__dirname, '..', '..', 'bot', 'telegram', 'commands')

module.exports = container => {
  const logger = container.logger.get()

  logger.debug('Init telegram bot...')

  const options = {}
  if (Number.parseInt(process.env.TELEGRAM_USE_WEB_HOOKS) !== 1) {
    options.polling = true
  }

  const bot = new TelegramBot(process.env.TELEGRAM_API_TOKEN, options)

  logger.info('Telegram bot is created')

  return {
    bot,
    initCommands () {
      logger.debug(`initCommands method call`)
      fs
        .readdirSync(COMMANDS_DIR)
        .filter(filename => filename.endsWith('.js'))
        .forEach(filename => {
          logger.debug(`Load commands from file: ${filename}`)
          require(path.join(COMMANDS_DIR, filename))(container, bot)
        })
      logger.info('Telegram commands are loaded')
      return this
    },
    async setWebHook ({app}) {
      logger.debug(`setWebHook method call. Use web hooks: ${process.env.TELEGRAM_USE_WEB_HOOKS}`)
      if (parseInt(process.env.TELEGRAM_USE_WEB_HOOKS) === 1) {
        const webHookUrl = `${process.env.APP_URL}${process.env.TELEGRAM_WEB_HOOK_URL}${process.env.TELEGRAM_API_TOKEN}`
        logger.info(`WebHook url: ${webHookUrl}. Get current WebHookInfo...`)
        await bot.deleteWebHook()
        const webHookInfo = await bot.getWebHookInfo()
        logger.info(`WebHook info: ${JSON.stringify(webHookInfo)}`)
        app.post(`${process.env.TELEGRAM_WEB_HOOK_URL}${process.env.TELEGRAM_API_TOKEN}`, (req, res) => {
          logger.debug('Get update from telegram via webhook')
          bot.processUpdate(req.body)
          res.sendStatus(200)
        })
        if (webHookInfo.url !== webHookUrl) {
          const setWebHookResult = await bot.setWebHook(webHookUrl)
          logger.info(`setWebHookResult = ${setWebHookResult}`)
        }
        logger.info(`Telegram web hook is set`)
      }
    }
  }
}
