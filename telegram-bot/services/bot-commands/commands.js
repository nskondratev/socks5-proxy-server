import { USER_STATE } from "../constants.js"
import { getChatIdAndUserName } from "../../utils/utils.js"

export default function ({ bot, logger, store }) {
    bot.onText(/\/start(.*)/, async (msg, _match) => {
        const { chatId, username } = getChatIdAndUserName(msg)

        logger.debug(`Received start message from @${username}`)

        try {
            if (!await store.isAdmin(username)) {
                await bot.sendMessage(chatId, 'Hello! You are not an admin of this proxy server.')

                return
            }

            const userState = { state: USER_STATE.IDLE, data: {} }

            await Promise.all([
                bot.sendMessage(chatId, 'Hello! You can manage proxy server.'),
                store.setUserState(username, userState),
                store.updateAdminChatId(username, chatId)
            ])
        } catch (err) {
            logger.error(err)
        }
    })

    bot.onText(/\/users_stats(.*)/, async (msg, _match) => {
        const { chatId, username } = getChatIdAndUserName(msg)

        logger.debug(`Received stats request from @${username}`)
        try {
            if (!await store.isAdmin(username)) {
                await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')

                return
            }

            const dataUsage = await store.getUsersStats()

            let message = '<b>Data usage by users:</b>\n\n'

            if (dataUsage.length > 0) {
                dataUsage.forEach(u => {
                    message += `<b>${u[0]}.</b> ${u[1]} (${moment(u[4]).fromNow()}): ${u[3]}\n`
                })
            } else {
                message += 'No usage stats.'
            }

            await Promise.all([
                bot.sendMessage(chatId, message, {
                    parse_mode: 'HTML',
                    reply_markup: { remove_keyboard: true }
                }),
                store.setUserState(username, { state: USER_STATE.IDLE, data: {} })
            ])
        } catch (err) {
            logger.error(err)
        }
    })

    bot.onText(/\/create_user(.*)/, async (msg, match) => {
        const { chatId, username } = getChatIdAndUserName(msg)

        logger.debug(`Received create user request from @${username}`)

        try {
            logger.debug(`Match: ${JSON.stringify(match)}`)
            if (!await util.isAdmin(username)) {
                await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')

                return
            }

            const userState = { state: USER_STATE.CREATE_USER_ENTER_USERNAME, data: {} }

            await Promise.all([
                bot.sendMessage(chatId, 'Enter username for the new proxy user.', {
                    reply_markup: { remove_keyboard: true }
                }),
                store.setUserState(username, userState)
            ])
        } catch (err) {
            logger.error(err)
            await bot.sendMessage(chatId, err.message)
        }
    })
}
