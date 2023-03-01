const moment = require('moment')
const passwordGenerator = require('generate-password')
const _ = require('lodash')

module.exports = (container, bot) => {
  const logger = container.logger.get()
  const util = container.util
  const USER_STATE = container.constants.userState

  bot.onText(/\/delete_user(.*)/, async (msg, match) => {
    const { chatId, username } = util.getChatIdAndUserName(msg)
    logger.debug(`Received create user request from @${username}`)
    try {
      if (!await util.isAdmin(username)) {
        await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')
      } else {
        const userState = { state: USER_STATE.DELETE_USER_ENTER_USERNAME, data: {} }
        await util.setUserState(username, userState)
        await bot.sendMessage(chatId, 'Enter username to delete.', { reply_markup: { remove_keyboard: true } })
      }
    } catch (err) {
      logger.error(err)
      await bot.sendMessage(chatId, err.message)
    }
  })

  bot.onText(/\/get_users(.*)/, async (msg, match) => {
    const { chatId, username } = util.getChatIdAndUserName(msg)
    logger.debug(`Received get users request from @${username}`)
    try {
      if (!await util.isAdmin(username)) {
        await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')
      } else {
        await util.setUserState(username, { state: USER_STATE.IDLE, data: {} })
        const users = await util.getUsers()
        let message = 'No users.'
        if (users.length) {
          message = '<b>Users</b>:\n\n'
          users.sort().forEach((u, i) => {
            message += `${i + 1}. ${u}\n`
          })
          message += `\n<b>Total: ${users.length}</b>`
        }
        await bot.sendMessage(chatId, message, { parse_mode: 'HTML', reply_markup: { remove_keyboard: true } })
      }
    } catch (err) {
      logger.error(err)
      await bot.sendMessage(chatId, err.message, { reply_markup: { remove_keyboard: true } })
    }
  })

  bot.onText(/\/generate_pass(.*)/, async (msg, match) => {
    const { chatId } = util.getChatIdAndUserName(msg)
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
  bot.onText(/^[^\/].*/, async (msg, match) => {
    const { chatId, username } = util.getChatIdAndUserName(msg)
    try {
      const userState = await util.getUserState(username)
      if (_.isNull(userState)) {
        logger.debug('User state is idle')
      } else {
        switch (userState.state) {
          case USER_STATE.IDLE:
            await bot.sendMessage(chatId, 'Enter command')
            break
          case USER_STATE.CREATE_USER_ENTER_USERNAME: {
            const proxyUsername = msg.text.trim()
            logger.debug(`Entered username: ${proxyUsername}`)
            if (!proxyUsername) {
              await bot.sendMessage(chatId, 'Username can not be empty. Enter the new one.')
            } else {
              if (await util.isUsernameFree(proxyUsername)) {
                userState.state = USER_STATE.CREATE_USER_ENTER_PASSWORD
                userState.data.username = proxyUsername
                const suggestedPassword = passwordGenerator.generate({
                  length: 10,
                  numbers: true,
                  uppercase: true,
                  strict: true
                })
                await util.setUserState(username, userState)
                await bot.sendMessage(chatId, 'Ok. Enter the password or use the suggested one.', {
                  reply_markup: {
                    keyboard: [[suggestedPassword]]
                  }
                })
              } else {
                await bot.sendMessage(chatId, 'This username is already taken. Enter another one.')
              }
            }
            break
          }
          case USER_STATE.CREATE_USER_ENTER_PASSWORD: {
            const proxyPassword = msg.text.trim()
            if (!proxyPassword) {
              await bot.sendMessage(chatId, 'Password can not be empty. Enter the new one.')
            } else {
              await util.createUser(userState.data.username, proxyPassword)
              await util.setUserState(username, { state: USER_STATE.IDLE, data: {} })
              const message = `User created. Send this settings to him:\n\n<b>host:</b> ${process.env.PUBLIC_URL.replace('https://', '').replace('http://', '')}\n<b>port:</b> ${process.env.APP_PORT}\n<b>username:</b> ${userState.data.username}\n<b>password:</b> ${proxyPassword}`
              await bot.sendMessage(chatId, message, { parse_mode: 'HTML', reply_markup: { remove_keyboard: true } })
            }
            break
          }
          case USER_STATE.DELETE_USER_ENTER_USERNAME: {
            const usernameToDelete = msg.text.trim()
            logger.debug(`Entered username: ${usernameToDelete}`)
            if (!await util.isUsernameFree(usernameToDelete)) {
              await util.deleteUser(usernameToDelete)
              await util.setUserState(username, { state: USER_STATE.IDLE, data: {} })
              await bot.sendMessage(chatId, 'User deleted.')
            } else {
              await bot.sendMessage(chatId, 'User with provided username does not exists. Enter another one.')
            }
            break
          }
        }
      }
    } catch
    (err) {
      logger.error(err)
      await bot.sendMessage(chatId, err.message, { reply_markup: { remove_keyboard: true } })
    }
  }
  )
}
