import { existsSync } from 'node:fs'
import { join } from 'node:path'

import { hash } from 'bcrypt'
import bluebird from 'bluebird'
import chalk from 'chalk'
import dotenv from 'dotenv'
import figlet from 'figlet'
import inquirer from 'inquirer'

import redis from '../services/redis.js'
import { REDIS } from '../services/constants.js'
import { dirname } from '../utils/dirname.js'

const figletPromise = bluebird.promisify(figlet)

if (existsSync(join(dirname(import.meta.url), '..', '.env'))) {
  dotenv.config()
}

const createUser = async data => {
  const userPassword = await redis.hGet(REDIS.AUTH_USER_KEY, data.username)
  if (userPassword) {
    throw new Error('User with provided username already exists')
  }
  const hashedPassword = await hash(data.password, 10)
  await redis.hSet(REDIS.AUTH_USER_KEY, data.username, hashedPassword)
}

const logo = await figletPromise('Create user', { font: 'Standard' })

console.log(chalk.blueBright(logo))

try {
  const answers = await inquirer.prompt([
    {
      type: 'input',
      name: 'username',
      message: 'Input username and press Enter:'
    },
    {
      type: 'password',
      name: 'password',
      message: 'Input password and press Enter to create new user:'
    }
  ])

  await createUser(answers)

  console.log(chalk.green('User successfully created.'))
  process.exit(0)
} catch (err) {
  console.log(chalk.red(err))
  process.exit(1)
}
