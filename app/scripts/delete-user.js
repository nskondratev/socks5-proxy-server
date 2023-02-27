import fs from 'node:fs'
import path from 'node:path'

import Promise from 'bluebird'
import chalk from 'chalk'
import dotenv from 'dotenv'
import figlet from 'figlet'
import inquirer from 'inquirer'

import redis from '../services/redis.js'
import { REDIS } from '../services/constants.js'
import { dirname } from '../utils/dirname.js'

const figletPromise = Promise.promisify(figlet)

if (fs.existsSync(path.join(dirname(import.meta.url), '..', '.env'))) {
  dotenv.config()
}

const deleteUser = async ({ username }) => {
  const userPassword = await redis.hGet(REDIS.AUTH_USER_KEY, username)
  if (!userPassword) {
    throw new Error('User with provided username not found')
  }

  await redis.hDel(REDIS.AUTH_USER_KEY, username)
}

const logo = await figletPromise('Delete user', { font: 'Standard' })

console.log(chalk.blueBright(logo))

try {
  const answers = await inquirer.prompt([
    {
      type: 'input',
      name: 'username',
      message: 'Input username and press Enter:'
    }
  ])

  await deleteUser(answers)

  console.log(chalk.green('User successfully deleted.'))
  process.exit(0)
} catch (err) {
  console.log(chalk.red(err))
  process.exit(1)
}
