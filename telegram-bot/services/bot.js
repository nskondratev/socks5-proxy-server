import path from 'node:path'

import TelegramBot from 'node-telegram-bot-api'
import Socks5AgentClass from 'socks5-https-client/lib/Agent'

import { dirname } from './utils.js'
import initCommands from './commands.js'

export class TelegramBot {
    #store
    #logger
    #bot

    constructor(store, logger) {
        this.#store = store
        this.#logger = logger

        this.#initBot()

        initCommands({
            bot: this.#bot,
            logger: this.#logger,
            store: this.#store,
        })
    }

    #initBot() {
        this.#logger.debug('Init telegram bot...')

        const options = {}
        if (process.env.PROXY_SOCKS5_HOST) {
            options.request = {
                agentClass: Socks5AgentClass,
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
                key: path.join(dirname(import.meta.url), '..', '..', 'ssl', 'key.pem'),
                cert: path.join(dirname(import.meta.url), '..', '..', 'ssl', 'crt.pem')
            }
        }

        const bot = new TelegramBot(process.env.TELEGRAM_API_TOKEN, options)

        this.#logger.info('Telegram bot is created')

        if (parseInt(process.env.TELEGRAM_USE_WEB_HOOKS) === 1) {
            const setWebHook = async () => {
                const webHookUrl = `${process.env.PUBLIC_URL}${process.env.TELEGRAM_WEB_HOOK_URL}${process.env.TELEGRAM_API_TOKEN}`
                this.#logger.info(`WebHook url: ${webHookUrl}. Get current WebHookInfo...`)

                await bot.deleteWebHook()
                const webHookInfo = await bot.getWebHookInfo()
                this.#logger.info(`WebHook info: ${JSON.stringify(webHookInfo)}`)
                if (webHookInfo.url !== webHookUrl) {
                    const setWebHookResult = await bot.setWebHook(webHookUrl, {
                        certificate: options.webHook.cert
                    })
                    this.#logger.info(`setWebHookResult = ${setWebHookResult}`)
                }
                this.#logger.info('Telegram web hook is set')
            }

            setWebHook().catch(err => this.#logger.error(err))
        }

        bot.on('polling_error', err => this.#logger.error(err))

        bot.on('webhook_error', err => this.#logger.error(err))

        this.#bot = bot
    }
}
