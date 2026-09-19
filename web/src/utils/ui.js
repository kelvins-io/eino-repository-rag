import { Modal } from '@arco-design/web-vue'
import { t } from '@/i18n'

export function confirmAction(content, title) {
  return new Promise((resolve, reject) => {
    let settled = false
    const finish = (ok) => {
      if (settled) return
      settled = true
      if (ok) resolve()
      else reject(new Error('cancel'))
    }
    Modal.confirm({
      title: title || t('common.confirm'),
      content,
      okText: t('common.confirm'),
      cancelText: t('common.cancel'),
      onOk: () => finish(true),
      onCancel: () => finish(false),
    })
  })
}
