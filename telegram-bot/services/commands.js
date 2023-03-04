import { USER_STATE } from "./constants.js"
import { getChatIdAndUserName } from "./utils.js"

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
            await store.setUserState(username, userState)

            await Promise.all([
                bot.sendMessage(chatId, 'Hello! You can manage proxy server.'),
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

            await store.setUserState(username, { state: USER_STATE.IDLE, data: {} })
            await bot.sendMessage(chatId, message, {
                parse_mode: 'HTML',
                reply_markup: { remove_keyboard: true }
            })
        } catch (err) {
            logger.error(err)
        }
    })

    bot.onText(/\/create_user(.*)/, async (msg, match) => {
        const { chatId, username } = getChatIdAndUserName(msg)

        logger.debug(`Received create user request from @${username}`)

        try {
            logger.debug(`Match: ${JSON.stringify(match)}`)
            if (!await store.isAdmin(username)) {
                await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')

                return
            }

            const userState = { state: USER_STATE.CREATE_USER_ENTER_USERNAME, data: {} }

            await store.setUserState(username, userState)
            await bot.sendMessage(chatId, 'Enter username for the new proxy user.', {
                reply_markup: { remove_keyboard: true }
            })
        } catch (err) {
            logger.error(err)
            await bot.sendMessage(chatId, err.message)
        }
    })

    bot.onText(/\/delete_user(.*)/, async (msg, _match) => {
        const { chatId, username } = getChatIdAndUserName(msg)

        logger.debug(`Received create user request from @${username}`)

        try {
            if (!await store.isAdmin(username)) {
                await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')

                return
            }

            const userState = { state: USER_STATE.DELETE_USER_ENTER_USERNAME, data: {} }
            await store.setUserState(username, userState)
            await bot.sendMessage(chatId, 'Enter username to delete.', {
                reply_markup: { remove_keyboard: true }
            })
        } catch (err) {
            logger.error(err)
            await bot.sendMessage(chatId, err.message)
        }
    })

    bot.onText(/\/get_users(.*)/, async (msg, _match) => {
        const { chatId, username } = getChatIdAndUserName(msg)

        logger.debug(`Received get users request from @${username}`)

        try {
            if (!await store.isAdmin(username)) {
                await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')

                return
            }

            await store.setUserState(username, { state: USER_STATE.IDLE, data: {} })
            const users = await store.getUsers()

            let message = 'No users.'

            if (users.length > 0) {
                message = '<b>Users</b>:\n\n'

                users.sort().forEach((u, i) => {
                    message += `${i + 1}. ${u}\n`
                })

                message += `\n<b>Total: ${users.length}</b>`
            }

            await bot.sendMessage(chatId, message, {
                parse_mode: 'HTML',
                reply_markup: { remove_keyboard: true }
            })
        } catch (err) {
            logger.error(err)
            await bot.sendMessage(chatId, err.message, { reply_markup: { remove_keyboard: true } })
        }
    })

    bot.onText(/\/generate_pass(.*)/, async (msg, match) => {
        const { chatId } = getChatIdAndUserName(msg)

        try {
            const length = parseInt(match[1].trim()) || 10

            await bot.sendMessage(chatId, passwordGenerator.generate({
                length,
                numbers: true,
                uppercase: true,
                strict: true
            }))
        } catch (err) {
            logger.error(err)
        }
    })

    // eslint-disable-next-line
    bot.onText(/^[^\/].*/, async (msg, _match) => {
        const { chatId, username } = getChatIdAndUserName(msg)

        try {
            const userState = await store.getUserState(username)

            if (_.isNull(userState)) {
                logger.debug('User state is idle')

                return
            }

            switch (userState.state) {
                case USER_STATE.IDLE:
                    await bot.sendMessage(chatId, 'Enter command')
                    break
                case USER_STATE.CREATE_USER_ENTER_USERNAME: {
                    const proxyUsername = msg.text.trim()

                    logger.debug(`Entered username: ${proxyUsername}`)

                    if (!proxyUsername) {
                        await bot.sendMessage(chatId, 'Username can not be empty. Enter the new one.')

                        break
                    }

                    if (!await store.isUsernameFree(proxyUsername)) {
                        await bot.sendMessage(chatId, 'This username is already taken. Enter another one.')

                        break
                    }

                    userState.state = USER_STATE.CREATE_USER_ENTER_PASSWORD
                    userState.data.username = proxyUsername

                    const suggestedPassword = passwordGenerator.generate({
                        length: 10,
                        numbers: true,
                        uppercase: true,
                        strict: true
                    })

                    await store.setUserState(username, userState)
                    await bot.sendMessage(chatId, 'Ok. Enter the password or use the suggested one.', {
                        reply_markup: {
                            keyboard: [[suggestedPassword]]
                        }
                    })

                    break
                }
                case USER_STATE.CREATE_USER_ENTER_PASSWORD: {
                    const proxyPassword = msg.text.trim()

                    if (!proxyPassword) {
                        await bot.sendMessage(chatId, 'Password can not be empty. Enter the new one.')

                        break
                    }

                    await store.createUser(userState.data.username, proxyPassword)
                    await store.setUserState(username, { state: USER_STATE.IDLE, data: {} })

                    const message = `User created. Send this settings to him:\n\n<b>host:</b> ${process.env.PUBLIC_URL.replace('https://', '').replace('http://', '')}\n<b>port:</b> ${process.env.APP_PORT}\n<b>username:</b> ${userState.data.username}\n<b>password:</b> ${proxyPassword}`

                    await bot.sendMessage(chatId, message, {
                        parse_mode: 'HTML',
                        reply_markup: { remove_keyboard: true }
                    })

                    break
                }
                case USER_STATE.DELETE_USER_ENTER_USERNAME: {
                    const usernameToDelete = msg.text.trim()

                    logger.debug(`Entered username: ${usernameToDelete}`)

                    if (await store.isUsernameFree(usernameToDelete)) {
                        await bot.sendMessage(chatId, 'User with provided username does not exists. Enter another one.')

                        break
                    }

                    await store.deleteUser(usernameToDelete)
                    await store.setUserState(username, { state: USER_STATE.IDLE, data: {} })
                    await bot.sendMessage(chatId, 'User deleted.')

                    break
                }
            }
        } catch (err) {
            logger.error(err)
            await bot.sendMessage(chatId, err.message, { reply_markup: { remove_keyboard: true } })
        }
    })
}
