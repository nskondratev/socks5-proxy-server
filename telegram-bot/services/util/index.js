const Promise = require('bluebird')
const bcrypt = Promise.promisifyAll(require('bcrypt'))
const _ = require('lodash')

module.exports = container => {
  const redis = container.redis
  const REDIS = container.constants.redis

  return {
    async isAdmin (username) {
      return !!await redis.hExists(REDIS.ADMIN_USER_KEY, username)
    },

    async createAdmin (username) {
      return redis.hSet(REDIS.ADMIN_USER_KEY, username, 1)
    },

    async updateAdminChatId (username, chatId) {
      return redis.hSet(REDIS.ADMIN_USER_KEY, username, chatId)
    },

    async deleteAdmin (username) {
      return redis.hDel(REDIS.ADMIN_USER_KEY, username)
    },

    async getUsersStats () {
      const parseStatsResults = (data, lastLoginData) => {
        const users = []
        if (data) {
          Object.keys(data).forEach(username => users.push({ username, usage: parseInt(data[username]) }))
          users.sort((a, b) => b.usage - a.usage)
          return users.map((u, i) => {
            let usage = `${u.usage} B`
            if (u.usage > 1073741824) {
              // Usage in GB
              usage = `${(u.usage / 1073741824).toFixed(2)} GB`
            } else if (u.usage > 1048576) {
              // Usage in MB
              usage = `${(u.usage / 1048576).toFixed(2)} MB`
            } else if (u.usage > 1024) {
              // Usage in KB
              usage = `${(u.usage / 1024).toFixed(2)} KB`
            }
            return [i + 1, ...Object.values(u), usage, lastLoginData[u.username] || '-']
          })
        }
        return users
      }
      let [dataUsage, lastLogin] = await Promise.all([
        redis.hGetAll(REDIS.DATA_USAGE_KEY),
        redis.hGetAll(REDIS.AUTH_DATE_KEY)
      ])
      dataUsage = parseStatsResults(dataUsage, lastLogin)
      return dataUsage
    },

    async createUser (username, password) {
      const userPassword = await redis.hGet(REDIS.AUTH_USER_KEY, username)
      if (userPassword) {
        throw new Error('User with provided username already exists')
      }
      const hashedPassword = await bcrypt.hashAsync(password, 10)
      await redis.hSet(REDIS.AUTH_USER_KEY, username, hashedPassword)
    },

    async deleteUser (username) {
      const userPassword = await redis.hGet(REDIS.AUTH_USER_KEY, username)
      if (!userPassword) {
        throw new Error('User with provided username not found')
      }
      await redis.hDel(REDIS.AUTH_USER_KEY, username)
    },

    async getUserState (username) {
      const userState = await redis.hGet(REDIS.USER_STATE, username)
      return _.isString(userState) ? JSON.parse(userState) : userState
    },

    async setUserState (username, state) {
      return redis.hSet(REDIS.USER_STATE, username, JSON.stringify(state))
    },

    getChatIdAndUserName (msg) {
      return { username: msg.from.username, chatId: msg.chat.id }
    },

    async isUsernameFree (username) {
      return !await redis.hExists(REDIS.AUTH_USER_KEY, username)
    },

    async getUsers () {
      return redis.hKeys(REDIS.AUTH_USER_KEY)
    }
  }
}
