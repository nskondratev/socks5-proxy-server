import bcrypt from 'bcrypt'
import Promise from 'bluebird'
import _ from 'lodash'
import { createClient } from 'redis'

import { REDIS } from './constants.js'

const bcryptPromise = Promise.promisifyAll(bcrypt)

const redisClient = createClient({
    host: process.env.REDIS_HOST,
    port: process.env.REDIS_PORT,
    db: process.env.REDIS_DB
})

export const client = await redisClient.connect().then(() => redisClient)

export class Store {
    #redis

    constructor(redis) {
        this.#redis = redis
    }

    async isAdmin(username) {
        return !!await this.#redis.hExists(REDIS.ADMIN_USER_KEY, username)
    }

    async createAdmin(username) {
        return this.#redis.hSet(REDIS.ADMIN_USER_KEY, username, 1)
    }

    async updateAdminChatId(username, chatId) {
        return this.#redis.hSet(REDIS.ADMIN_USER_KEY, username, chatId)
    }

    async deleteAdmin(username) {
        return this.#redis.hDel(REDIS.ADMIN_USER_KEY, username)
    }

    #parseStatsResults(data, lastLoginData) {
        if (Object.keys(data).length === 0) {
            return []
        }

        const users = []

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

    async getUsersStats() {
        let [dataUsage, lastLogin] = await Promise.all([
            this.#redis.hGetAll(REDIS.DATA_USAGE_KEY),
            this.#redis.hGetAll(REDIS.AUTH_DATE_KEY)
        ])

        return this.#parseStatsResults(dataUsage, lastLogin)
    }

    async createUser(username, password) {
        const userPassword = await this.#redis.hGet(REDIS.AUTH_USER_KEY, username)
        if (userPassword) {
            throw new Error('User with provided username already exists')
        }
        const hashedPassword = await bcryptPromise.hashAsync(password, 10)
        await this.#redis.hSet(REDIS.AUTH_USER_KEY, username, hashedPassword)
    }

    async deleteUser(username) {
        const userPassword = await this.#redis.hGet(REDIS.AUTH_USER_KEY, username)
        if (!userPassword) {
            throw new Error('User with provided username not found')
        }

        await this.#redis.hDel(REDIS.AUTH_USER_KEY, username)
    }

    async getUserState(username) {
        const userState = await this.#redis.hGet(REDIS.USER_STATE, username)
        return _.isString(userState) ? JSON.parse(userState) : userState
    }

    async setUserState(username, state) {
        return this.#redis.hSet(REDIS.USER_STATE, username, JSON.stringify(state))
    }

    async isUsernameFree(username) {
        return !await this.#redis.hExists(REDIS.AUTH_USER_KEY, username)
    }

    async getUsers() {
        return this.#redis.hKeys(REDIS.AUTH_USER_KEY)
    }
}
