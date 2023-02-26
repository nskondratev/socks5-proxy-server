const TelegramBot = require('node-telegram-bot-api')
const path = require('path')
const fs = require('fs')

const COMMANDS_DIR = path.join(__dirname, '..', '..', 'bot-commands')

module.exports = container => {
  const logger = container.logger.get()

  logger.debug('Init telegram bot...')

  const options = {}
  if (process.env.PROXY_SOCKS5_HOST) {
    options.request = {
      agentClass: require('socks5-https-client/lib/Agent'),
      agentOptions: {
        socksHost: process.env.PROXY_SOCKS5_HOST
      }
    }
    if (process.env.PROXY_SOCKS5_PORT) {
      options.request.agentOptions.socksPort = parseInt(process.env.PROXY_SOCKS5_PORT)
    }
    if (process.env.PROXY_SOCKS5_USERNAME) {
      options.request.agentOptions.socksUsername = process.env.PROXY_SOCKS5_USERNAME
    }
    if (process.env.PROXY_SOCKS5_PASSWORD) {
      options.request.agentOptions.socksPassword = process.env.PROXY_SOCKS5_PASSWORD
    }
  }

  if (Number.parseInt(process.env.TELEGRAM_USE_WEB_HOOKS) !== 1) {
    options.polling = true
  } else {
    options.webHook = {
      port: parseInt(process.env.BOT_APP_PORT),
      key: path.join(__dirname, '..', '..', 'ssl', 'key.pem'),
      cert: path.join(__dirname, '..', '..', 'ssl', 'crt.pem')
    }
  }

  const bot = new TelegramBot(process.env.TELEGRAM_API_TOKEN, options)

  logger.info('Telegram bot is created')

  if (parseInt(process.env.TELEGRAM_USE_WEB_HOOKS) === 1) {
    const setWebHook = async () => {
      const webHookUrl = `${process.env.PUBLIC_URL}${process.env.TELEGRAM_WEB_HOOK_URL}${process.env.TELEGRAM_API_TOKEN}`
      logger.info(`WebHook url: ${webHookUrl}. Get current WebHookInfo...`)
      await bot.deleteWebHook()
      const webHookInfo = await bot.getWebHookInfo()
      logger.info(`WebHook info: ${JSON.stringify(webHookInfo)}`)
      if (webHookInfo.url !== webHookUrl) {
        const setWebHookResult = await bot.setWebHook(webHookUrl, {
          certificate: options.webHook.cert
        })
        logger.info(`setWebHookResult = ${setWebHookResult}`)
      }
      logger.info('Telegram web hook is set')
    }
    setWebHook().catch(err => logger.error(err))
  }

  bot.on('polling_error', err => logger.error(err))

  bot.on('webhook_error', err => logger.error(err))

  return {
    bot,
    initCommands () {
      logger.debug('initCommands method call')
      fs
        .readdirSync(COMMANDS_DIR)
        .filter(filename => filename.endsWith('.js'))
        .forEach(filename => {
          logger.debug(`Load commands from file: ${filename}`)
          require(path.join(COMMANDS_DIR, filename))(container, bot)
        })
      logger.info('Telegram commands are loaded')
      return this
    }
  }
}
