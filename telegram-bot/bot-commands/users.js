module.exports = (container, bot) => {
  const logger = container.logger.get()
  const util = container.util

  bot.onText(/\/users_stats(.*)/, async (msg, match) => {
    const chatId = msg.chat.id
    const username = msg.from.username
    logger.debug(`Received stats request from @${username}`)
    try {
      if (!await util.isAdmin(username)) {
        await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')
      } else {
        const dataUsage = await util.getUsersStats()
        let message = `*Data usage by users:*\n\n`
        dataUsage.forEach(u => {
          message += `*${u[0]}.* ${u[1]} (${u[4]}): ${u[3]}\n`
        })
        await bot.sendMessage(chatId, message, {parse_mode: 'Markdown'})
      }
    } catch (err) {
      logger.error(err)
    }
  })

  bot.onText(/\/create_user (.*) (.*)/, async (msg, match) => {
    const chatId = msg.chat.id
    const username = msg.from.username
    logger.debug(`Received create user request from @${username}`)
    try {
      if (!await util.isAdmin(username)) {
        await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')
      } else {
        const proxyUsername = match[1]
        const proxyPassword = match[2]
        await util.createUser(proxyUsername, proxyPassword)
        await bot.sendMessage(chatId, 'Proxy user created')
      }
    } catch (err) {
      logger.error(err)
      await bot.sendMessage(chatId, err.message)
    }
  })

  bot.onText(/\/delete_user (.*)/, async (msg, match) => {
    const chatId = msg.chat.id
    const username = msg.from.username
    logger.debug(`Received create user request from @${username}`)
    try {
      if (!await util.isAdmin(username)) {
        await bot.sendMessage(chatId, 'Sorry, this functionality is available only for admin users.')
      } else {
        const proxyUsername = match[1]
        await util.deleteUser(proxyUsername)
        await bot.sendMessage(chatId, 'Proxy user deleted')
      }
    } catch (err) {
      logger.error(err)
      await bot.sendMessage(chatId, err.message)
    }
  })
}
