const Promise = require('bluebird')
const bcrypt = Promise.promisifyAll(require('bcrypt'))
const inquirer = require('inquirer')
const figlet = Promise.promisify(require('figlet'))
const chalk = require('chalk')

const fs = require('fs')
const path = require('path')

if (fs.existsSync(path.join(__dirname, '..', '.env'))) {
  require('dotenv').config()
}

const container = require('../services')

const redis = container.cradle.redis
const CONSTANTS = container.cradle.constants

const createUser = async data => {
  const userPassword = await redis.hgetAsync(CONSTANTS.REDIS.AUTH_USER_KEY, data.username)
  if (userPassword) {
    throw new Error('User with provided username already exists')
  }
  const hashedPassword = await bcrypt.hashAsync(data.password, 10)
  await redis.hsetAsync(CONSTANTS.REDIS.AUTH_USER_KEY, data.username, hashedPassword)
}

;(async () => {
  const logo = await figlet('Create user', {font: 'Standard'})
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
})()
