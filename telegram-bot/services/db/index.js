const _ = require('lodash')
const Sequelize = require('sequelize')

const transHooks = Symbol('sequelize_transaction_hooks_property')   // used as an 'invisible' property on transaction objects, used to stored "after*" hook functions that should only run if the transaction actually commits successfully

/**
 * Function that call hookfn after transaction was successfully committed
 *
 * https://github.com/sequelize/sequelize/issues/8585
 *
 * @param transaction
 * @param hookfn
 * @returns {*}
 */
const afterTransactionCommit = (transaction, hookfn) => {
  if (!_.isFunction(hookfn)) return
  if (!transaction) return hookfn()

  if (!transaction[transHooks]) {
    transaction[transHooks] = []

    const origfn = transaction.commit

    transaction.commit = function (...args) {
      const prom = origfn.call(this, ...args),
        run_hooks = v => transaction[transHooks].forEach(fn => fn()) && v


      // the following is possibly an overly defensive check for a Promise ...
      return _.isFunction(prom && prom.then) ? prom.then(run_hooks) : run_hooks(prom)
    }
  }

  transaction[transHooks].push(hookfn)
}

const sequelize = new Sequelize(process.env.DB_DATABASE, process.env.DB_USER, process.env.DB_PASS, {
  host: process.env.DB_HOST,
  port: process.env.DB_PORT,
  dialect: process.env.DB_DIALECT,
  operatorsAliases: false
})

module.exports = () => ({sequelize, Sequelize, afterTransactionCommit})
