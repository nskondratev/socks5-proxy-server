const Promise = require('bluebird')
const bcrypt = Promise.promisifyAll(require('bcrypt'))
const _ = require('lodash')

module.exports = container => {
  const redis = container.redis
  const REDIS = container.constants.redis

  return {
    async isAdmin (username) {
      return !!parseInt(await redis.hexistsAsync(REDIS.ADMIN_USER_KEY, username))
    },

    async createAdmin (username) {
      return redis.hsetAsync(REDIS.ADMIN_USER_KEY, username, 1)
    },

    async updateAdminChatId (username, chatId) {
      return redis.hsetAsync(REDIS.ADMIN_USER_KEY, username, chatId)
    },

    async deleteAdmin (username) {
      return redis.hdelAsync(REDIS.ADMIN_USER_KEY, username)
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
        redis.hgetallAsync(REDIS.DATA_USAGE_KEY),
        redis.hgetallAsync(REDIS.AUTH_DATE_KEY)
      ])
      dataUsage = parseStatsResults(dataUsage, lastLogin)
      return dataUsage
    },

    async createUser (username, password) {
      const userPassword = await redis.hgetAsync(REDIS.AUTH_USER_KEY, username)
      if (userPassword) {
        throw new Error('User with provided username already exists')
      }
      const hashedPassword = await bcrypt.hashAsync(password, 10)
      await redis.hsetAsync(REDIS.AUTH_USER_KEY, username, hashedPassword)
    },

    async deleteUser (username) {
      const userPassword = await redis.hgetAsync(REDIS.AUTH_USER_KEY, username)
      if (!userPassword) {
        throw new Error('User with provided username not found')
      }
      await redis.batch([
        ['hdel', REDIS.AUTH_USER_KEY, username]
        // ['hdel', CONSTANTS.REDIS.DATA_USAGE_KEY, username] // Not remove data usage stats
      ]).execAsync()
    },

    async getUserState (username) {
      const userState = await redis.hgetAsync(REDIS.USER_STATE, username)
      return _.isString(userState) ? JSON.parse(userState) : userState
    },

    async setUserState (username, state) {
      return redis.hsetAsync(REDIS.USER_STATE, username, JSON.stringify(state))
    },

    getChatIdAndUserName (msg) {
      return { username: msg.from.username, chatId: msg.chat.id }
    },

    async isUsernameFree (username) {
      return !parseInt(await redis.hexistsAsync(REDIS.AUTH_USER_KEY, username))
    },

    async getUsers () {
      return redis.hkeysAsync(REDIS.AUTH_USER_KEY)
    }
  }
}
