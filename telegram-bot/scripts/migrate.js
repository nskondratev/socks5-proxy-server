const Promise = require('bluebird')
const fs = Promise.promisifyAll(require('fs'))
const path = require('path')

if (fs.existsSync(path.join(__dirname, '..', '.env'))) {
  require('dotenv').config()
}

const Umzug = require('umzug')

const chalk = require('chalk')
const args = require('yargs').argv
const container = require('../services')

const MIGRATIONS_DIR = path.join(__dirname, '..', 'migrations')
const SEEDERS_DIR = path.join(__dirname, '..', 'seeders')

const storage = 'sequelize'

// Up or down
let mode = args.undo ? 'down' : 'up'
let isMigration = !args.seed

const umzug = new Umzug({
  storage,
  storageOptions: {
    sequelize: container.cradle.db.sequelize
  },
  migrations: {
    path: isMigration ? MIGRATIONS_DIR : SEEDERS_DIR,
    params: [
      container.cradle.db.sequelize.getQueryInterface(),
      container.cradle.db.sequelize.constructor,
      container.cradle
    ]
  }
})

const runMigrations = () => new Promise((resolve, reject) => {
  if (mode === 'up') {
    umzug
      .up()
      .then(migrations => {
        migrations.forEach(m => console.log(chalk.green(`Migrated: ${m.file}`)))
        resolve()
      })
      .catch(reject)
  } else {
    umzug
      .down()
      .then(migrations => {
        console.log(chalk.green(`Migrations: ${migrations.map(m => m.file).join(', ')} are reverted.`))
        resolve()
      })
      .catch(reject)
  }
})

runMigrations()
  .then(() => {
    console.log(chalk.bgGreen.gray('All migrations are rolled'))
    process.exit(0)
  })
  .catch(err => {
    console.log(chalk.red(err))
    process.exit(1)
  })
