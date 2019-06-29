module.exports = (container, bot) => {
  const logger = container.logger.get()
  const util = container.util
  const USER_STATE = container.constants.userState

  bot.onText(/\/start(.*)/, async (msg, match) => {
    const chatId = msg.chat.id
    const username = msg.from.username
    logger.debug(`Received start message from @${username}`)
    try {
      const isAdmin = await util.isAdmin(username)
      let message = 'Hello! You are not an admin of this proxy server.'
      if (isAdmin) {
        message = 'Hello! You can manage proxy server.'
        const userState = { state: USER_STATE.IDLE, data: {} }
        await util.setUserState(username, userState)
        await util.updateAdminChatId(username, chatId)
      }
      await bot.sendMessage(chatId, message)
    } catch (err) {
      logger.error(err)
    }
  })
}
