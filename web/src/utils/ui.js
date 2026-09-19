import { Modal } from '@arco-design/web-vue'

export function confirmAction(content, title = '确认') {
  return new Promise((resolve, reject) => {
    let settled = false
    const finish = (ok) => {
      if (settled) return
      settled = true
      if (ok) resolve()
      else reject(new Error('cancel'))
    }
    Modal.confirm({
      title,
      content,
      okText: '确定',
      cancelText: '取消',
      onOk: () => finish(true),
      onCancel: () => finish(false),
    })
  })
}
