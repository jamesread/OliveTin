import './ActionButton.js' // To define action-button
import { ExecutionDialog } from './ExecutionDialog.js'
import { ActionStatusDisplay } from './ActionStatusDisplay.js'

function createElement (tag, attributes) {
  const el = document.createElement(tag)

  if (attributes !== null) {
    if (attributes.classNames !== undefined) {
      el.classList.add(...attributes.classNames)
    }

    if (attributes.innerText !== undefined) {
      el.innerText = attributes.innerText
    }
  }

  return el
}

function createTag (val) {
  const domTag = createElement('span', {
    innerText: val,
    classNames: ['tag']
  })

  return domTag
}

function createAnnotation (key, val) {
  const domAnnotation = createElement('span', {
    classNames: ['annotation']
  })

  domAnnotation.appendChild(createElement('span', {
    innerText: key,
    classNames: ['annotation-key']
  }))

  domAnnotation.appendChild(createElement('span', {
    innerText: val,
    classNames: ['annotation-value']
  }))

  return domAnnotation
}

/**
 * This is a weird function that just sets some globals.
 */
export function initMarshaller () {
  window.showSection = showSection
  window.showSectionView = showSectionView

  window.executionDialog = new ExecutionDialog()

  window.logEntries = new Map()
  window.registeredPaths = new Map()
  window.breadcrumbNavigation = []

  window.currentPath = ''

  window.addEventListener('EventExecutionStarted', onExecutionStarted)
  window.addEventListener('EventExecutionFinished', onExecutionFinished)
  window.addEventListener('EventOutputChunk', onOutputChunk)
}

function setUsername (username, provider) {
  document.getElementById('username').innerText = username
  document.getElementById('username').setAttribute('title', provider)

  if (window.settings.AuthLocalLogin || window.settings.AuthOAuth2Providers !== null) {
    if (username === 'guest') {
      document.getElementById('link-login').hidden = false
      document.getElementById('link-logout').hidden = true
    } else {
      document.getElementById('link-login').hidden = true

      if (provider === 'local' || provider === 'oauth2') {
        document.getElementById('link-logout').hidden = false
      }
    }
  }
}

export function marshalDashboardComponentsJsonToHtml (json) {
  if (json == null) { // eg: HTTP 403
    setUsername('guest', 'system')

    if (window.settings.AuthLoginUrl !== '') {
      window.location = window.settings.AuthLoginUrl
    } else {
      showSection('/login')
    }
  } else {
    setUsername(json.authenticatedUser, json.authenticatedUserProvider)

    marshalActionsJsonToHtml(json)
    marshalDashboardStructureToHtml(json)
  }

  document.body.setAttribute('initial-marshal-complete', 'true')
}

function marshalActionsJsonToHtml (json) {
  const currentIterationTimestamp = Date.now()

  window.actionButtons = {}

  for (const jsonButton of json.actions) {
    let htmlButton = window.actionButtons[jsonButton.id]

    if (typeof htmlButton === 'undefined') {
      htmlButton = document.createElement('action-button')
      htmlButton.constructFromJson(jsonButton)

      window.actionButtons[jsonButton.title] = htmlButton
    }

    htmlButton.updateFromJson(jsonButton)
    htmlButton.updateIterationTimestamp = currentIterationTimestamp
  }

  // Remove existing, but stale buttons (that were not updated in this round)
  for (const existingButton of document.querySelectorAll('action-button')) {
    if (existingButton.updateIterationTimestamp !== currentIterationTimestamp) {
      existingButton.remove()
    }
  }
}

function onOutputChunk (evt) {
  const chunk = evt.payload

  if (chunk.executionTrackingId === window.executionDialog.executionTrackingId) {
    window.terminal.write(chunk.output)

    window.executionDialog.showOutput()
  }
}

function onExecutionStarted (evt) {
  const logEntry = evt.payload.logEntry

  marshalLogsJsonToHtml({
    logs: [logEntry]
  })
}

function onExecutionFinished (evt) {
  const logEntry = evt.payload.logEntry

  window.logEntries.set(logEntry.executionTrackingId, logEntry)

  const actionButton = window.actionButtons[logEntry.actionTitle]

  if (actionButton === undefined) {
    return
  }

  switch (actionButton.popupOnStart) {
    case 'execution-button':
      if (document.querySelector('execution-button#execution-' + logEntry.executionTrackingId) !== null) { // If the button was created in our instance
        document.querySelector('execution-button#execution-' + logEntry.executionTrackingId).onExecutionFinished(logEntry)
      }
      break
    case 'execution-dialog-stdout-only':
    case 'execution-dialog':
      actionButton.onExecutionFinished(logEntry)

      // We don't need to fetch the logEntry for the dialog because we already
      // have it, so we open the dialog and it will get updated below.

      window.executionDialog.show()
      window.executionDialog.executionTrackingId = logEntry.uuid

      break
    default:
      actionButton.onExecutionFinished(logEntry)
      break
  }

  marshalLogsJsonToHtml({
    logs: [logEntry]
  })

  // If the current execution dialog is open, update that too
  if (window.executionDialog.dlg.open && window.executionDialog.executionUuid === logEntry.uuid) {
    window.executionDialog.renderExecutionResult({
      logEntry: logEntry
    })
  }
}

function convertPathToBreadcrumb (path) {
  const parts = path.split('/')

  const result = []

  for (let i = 0; i < parts.length; i++) {
    if (parts[i] === '') {
      continue
    }

    result.push(parts.slice(0, i + 1).join('/'))
  }

  return result
}

function showExecutionResult (pathName) {
  const executionTrackingId = pathName.split('/')[2]
  window.executionDialog.fetchExecutionResult(executionTrackingId)
  window.executionDialog.show()
}

function showSection (pathName) {
  if (pathName.startsWith('/logs/')) {
    showExecutionResult(pathName)
    pushNewNavigationPath(pathName)
    return
  }

  const path = window.registeredPaths.get(pathName)

  if (path === undefined) {
    console.warn('Section not found by path: ' + pathName)

    showSection('/')
    return
  }

  window.convertPathToBreadcrumb = convertPathToBreadcrumb
  window.currentPath = pathName
  window.breadcrumbNavigation = convertPathToBreadcrumb(pathName)

  for (const section of document.querySelectorAll('section')) {
    if (section.title === path.section) {
      section.style.display = 'block'
    } else {
      section.style.display = 'none'
    }
  }

  pushNewNavigationPath(pathName)

  setSectionNavigationVisible(false)

  showSectionView(path.view)
}

function pushNewNavigationPath (pathName) {
  window.history.pushState({
    path: pathName
  }, null, pathName)
}

function setSectionNavigationVisible (visible) {
  const nav = document.querySelector('nav')
  const btn = document.getElementById('sidebar-toggler-button')

  nav.removeAttribute('hidden')

  if (document.body.classList.contains('has-sidebar')) {
    if (visible) {
      btn.setAttribute('aria-pressed', false)
      btn.setAttribute('aria-label', 'Open sidebar navigation')
      btn.innerHTML = '&laquo;'

      nav.classList.add('shown')
    } else {
      btn.setAttribute('aria-pressed', true)
      btn.setAttribute('aria-label', 'Close sidebar navigation')
      btn.innerHTML = '&#9776;'

      nav.classList.remove('shown')
    }
  } else {
    btn.disabled = true
  }
}

export function setupSectionNavigation (style) {
  const nav = document.querySelector('nav')
  const btn = document.getElementById('sidebar-toggler-button')

  if (style === 'sidebar') {
    nav.classList.add('sidebar')

    document.body.classList.add('has-sidebar')

    btn.onclick = () => {
      if (nav.classList.contains('shown')) {
        setSectionNavigationVisible(false)
      } else {
        setSectionNavigationVisible(true)
      }
    }
  } else {
    nav.classList.add('topbar')

    document.body.classList.add('has-topbar')
  }

  registerSection('/', 'Actions', null, document.getElementById('showActions'))
  registerSection('/diagnostics', 'Diagnostics', null, document.getElementById('showDiagnostics'))
  registerSection('/logs', 'Logs', null, document.getElementById('showLogs'))
  registerSection('/login', 'Login', null, null)
}

function registerSection (path, section, view, linkElement) {
  window.registeredPaths.set(path, {
    section: section,
    view: view
  })

  if (linkElement != null) {
    addLinkToSection(path, linkElement)
  }
}

function addLinkToSection (pathName, element) {
  const path = window.registeredPaths.get(pathName)

  element.href = 'javascript:void(0)'
  element.title = path.section
  element.onclick = () => {
    showSection(pathName)
  }
}

export function refreshDiagnostics () {
  document.getElementById('diagnostics-sshfoundkey').innerHTML = window.settings.SshFoundKey
  document.getElementById('diagnostics-sshfoundconfig').innerHTML = window.settings.SshFoundConfig
}

function getSystemTitle (title) {
  return title.replaceAll(' ', '')
}

function marshalSingleDashboard (dashboard, nav) {
  const oldsection = document.querySelector('section[title="' + getSystemTitle(dashboard.title) + '"]')

  if (oldsection != null) {
    oldsection.remove()
  }

  const section = document.createElement('section')
  section.setAttribute('system-title', getSystemTitle(dashboard.title))
  section.title = section.getAttribute('system-title')

  const def = createFieldset('default', section)
  section.appendChild(def)

  document.getElementsByTagName('main')[0].appendChild(section)
  marshalContainerContents(dashboard, section, def, dashboard.title)

  const oldLi = nav.querySelector('li[title="' + dashboard.title + '"]')

  if (oldLi != null) {
    oldLi.remove()
  }

  const navigationA = document.createElement('a')
  navigationA.title = dashboard.title
  navigationA.innerText = dashboard.title

  registerSection('/' + getSystemTitle(section.title), section.title, null, navigationA)

  const navigationLi = document.createElement('li')
  navigationLi.appendChild(navigationA)
  navigationLi.title = dashboard.title

  document.getElementById('navigation-links').appendChild(navigationLi)
}

function marshalDashboardStructureToHtml (json) {
  const nav = document.getElementById('navigation-links')

  for (const dashboard of json.dashboards) {
    marshalSingleDashboard(dashboard, nav)
  }

  const rootGroup = document.querySelector('#root-group')

  for (const btn of Object.values(window.actionButtons)) {
    if (btn.parentElement === null) {
      rootGroup.appendChild(btn)
    }
  }

  const shouldHideActions = rootGroup.querySelectorAll('action-button').length === 0 && json.dashboards.length > 0

  if (shouldHideActions) {
    nav.querySelector('li[title="Actions"]').style.display = 'none'
  }

  if (window.currentPath !== '') {
    showSection(window.currentPath)
  } else if (window.location.pathname !== '/' && document.body.getAttribute('initial-marshal-complete') === null) {
    showSection(window.location.pathname)
  } else {
    if (shouldHideActions) {
      showSection('/' + getSystemTitle(json.dashboards[0].title))
    } else {
      showSection('/')
    }
  }
}

function marshalLink (item, fieldset) {
  let btn = window.actionButtons[item.title]

  if (typeof btn === 'undefined') {
    btn = document.createElement('button')
    btn.innerText = 'Action not found: ' + item.title
    btn.classList.add('error')
  }

  if (item.cssClass !== '') {
    btn.classList.add(item.cssClass)
  }

  fieldset.appendChild(btn)
}

function marshalMreOutput (dashboardComponent, fieldset) {
  const pre = document.createElement('pre')
  pre.classList.add('mre-output')
  pre.innerHTML = 'Waiting...'

  const executionStatus = {
    actionId: dashboardComponent.title
  }

  window.fetch(window.restBaseUrl + 'ExecutionStatus', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(executionStatus)
  }).then((res) => {
    if (res.ok) {
      return res.json()
    } else {
      pre.innerHTML = 'error'

      throw new Error(res.statusText)
    }
  }).then((json) => {
    updateMre(pre, json.logEntry)
  })

  const updateMre = (pre, json) => {
    pre.innerHTML = json.output
  }

  window.addEventListener('ExecutionFinished', (e) => {
    // The dashboard component "title" field is used for lots of things
    // and in this context for MreOutput it's just to refer an an actionId.
    //
    // So this is not a typo.
    if (e.payload.actionId === dashboardComponent.title) {
      updateMre(pre, e.payload)
    }
  })

  fieldset.appendChild(pre)
}

function marshalContainerContents (json, section, fieldset, parentDashboard) {
  for (const item of json.contents) {
    switch (item.type) {
      case 'fieldset':
        marshalFieldset(item, section, parentDashboard)
        break
      case 'directory': {
        const directoryPath = marshalDirectory(item, section)
        marshalDirectoryButton(item, fieldset, directoryPath)
      }
        break
      case 'display':
        marshalDisplay(item, fieldset)
        break
      case 'stdout-most-recent-execution':
        marshalMreOutput(item, fieldset)
        break
      case 'link':
        marshalLink(item, fieldset)
        break
      default:
    }
  }
}

function createFieldset (title, parentDashboard) {
  const legend = document.createElement('legend')
  legend.innerText = title

  const fs = document.createElement('fieldset')
  fs.title = title
  fs.appendChild(legend)

  if (typeof parentDashboard === 'undefined') {
    fs.setAttribute('parent-dashboard', '')
  } else {
    fs.setAttribute('parent-dashboard', parentDashboard)
  }

  return fs
}

function marshalFieldset (item, section, parentDashboard) {
  const fs = createFieldset(item.title, parentDashboard)

  marshalContainerContents(item, section, fs, parentDashboard)

  section.appendChild(fs)
}

function showSectionView (selected) {
  if (selected === '') {
    selected = null
  }

  for (const fieldset of document.querySelectorAll('fieldset')) {
    if (selected === null) {
      if ((fieldset.id === 'root-group' || fieldset.getAttribute('parent-dashboard') !== '') && fieldset.children.length > 1) {
        fieldset.style.display = 'grid'
      } else {
        fieldset.style.display = 'none'
      }
    } else {
      if (fieldset.title === selected) {
        fieldset.style.display = 'grid'
      } else {
        fieldset.style.display = 'none'
      }
    }
  }

  const current = window.registeredPaths.get(window.currentPath)

  for (const navLink of document.querySelector('nav').querySelectorAll('a')) {
    if (navLink.title === current.section) {
      navLink.classList.add('selected')
    } else {
      navLink.classList.remove('selected')
    }
  }

  rebuildH1BreadcrumbNavigation(selected)

  pushNewNavigationPath(window.currentPath)
}

function rebuildH1BreadcrumbNavigation () {
  const title = document.querySelector('h1')
  title.innerHTML = ''

  const rootLink = document.createElement('a')
  rootLink.innerText = window.pageTitle
  rootLink.href = 'javascript:void(0)'
  rootLink.onclick = () => {
    showSection('/')
  }

  title.appendChild(rootLink)

  for (const pathName of window.breadcrumbNavigation) {
    const sep = document.createElement('span')
    sep.innerHTML = ' &raquo; '
    title.append(sep)

    const path = window.registeredPaths.get(pathName)

    title.appendChild(createNavigationBreadcrumbDisplay(path))
  }

  document.title = title.innerText
}

function createNavigationBreadcrumbDisplay (path) {
  const a = document.createElement('a')
  a.href = 'javascript:void(0)'

  if (path.view === null) {
    a.title = path.section
    a.innerText = path.section
  } else {
    a.innerText = path.view
    a.title = path.view
  }

  a.onclick = () => {
    showSectionView(path.view)
  }

  return a
}

function marshalDisplay (item, fieldset) {
  const display = document.createElement('div')
  display.innerHTML = item.title
  display.classList.add('display')

  if (item.cssClass !== '') {
    display.classList.add(item.cssClass)
  }

  fieldset.appendChild(display)
}

function marshalDirectoryButton (item, fieldset, path) {
  const directoryButton = document.createElement('button')
  directoryButton.innerHTML = '<span class = "icon">' + item.icon + '</span> ' + item.title
  directoryButton.onclick = () => {
    showSection(path)
  }

  fieldset.appendChild(directoryButton)
}

function marshalDirectory (item, section) {
  const fs = createFieldset(item.title)
  fs.style.display = 'none'

  const directoryBackButton = document.createElement('button')
  directoryBackButton.innerHTML = window.settings.DefaultIconForBack
  directoryBackButton.title = 'Go back one directory'
  directoryBackButton.onclick = () => {
    showSection('/' + section.title)
  }

  fs.appendChild(directoryBackButton)

  marshalContainerContents(item, section, fs)

  section.appendChild(fs)

  const path = '/' + section.title + '/' + getSystemTitle(item.title)

  registerSection(path, section.title, item.title, null)

  return path
}

export function marshalLogsJsonToHtml (json) {
  // This function is called internally with a "fake" server response, that does
  // not have pageSize set. So we need to check if it's set before trying to use it.
  if (json.pageSize !== undefined) {
    document.getElementById('logs-server-page-size').innerText = json.pageSize
  }

  if (json.logs != null && json.logs.length > 0) {
    document.getElementById('logsTable').hidden = false
    document.getElementById('logsTableEmpty').hidden = true
  } else {
    return
  }

  for (const logEntry of json.logs) {
    let row = document.getElementById('log-' + logEntry.executionTrackingId)

    if (row == null) {
      const tpl = document.getElementById('tplLogRow')
      row = tpl.content.querySelector('tr').cloneNode(true)
      row.id = 'log-' + logEntry.executionTrackingId

      row.querySelector('.content').onclick = () => {
        window.executionDialog.reset()
        window.executionDialog.show()
        window.executionDialog.renderExecutionResult({
          logEntry: window.logEntries.get(logEntry.executionTrackingId)
        })
        pushNewNavigationPath('/logs/' + logEntry.executionTrackingId)
      }

      row.exitCodeDisplay = new ActionStatusDisplay(row.querySelector('.exit-code'))

      logEntry.dom = row

      window.logEntries.set(logEntry.executionTrackingId, logEntry)

      document.querySelector('#logTableBody').prepend(row)
    }

    row.querySelector('.timestamp').innerText = logEntry.datetimeStarted
    row.querySelector('.content').innerText = logEntry.actionTitle
    row.querySelector('.icon').innerHTML = logEntry.actionIcon
    row.setAttribute('title', logEntry.actionTitle)

    row.exitCodeDisplay.update(logEntry)

    row.querySelector('.tags').innerHTML = ''

    for (const tag of logEntry.tags) {
      row.querySelector('.tags').append(createTag(tag))
    }

    row.querySelector('.tags').append(createAnnotation('user', logEntry.user))
  }
}

window.addEventListener('popstate', (e) => {
  e.preventDefault()

  if (e.state != null && typeof e.state.path !== 'undefined') {
    showSection(e.state.path)
  }
})

export function refreshServerConnectionLabel () {
  if (window.restAvailable) {
    document.querySelector('#serverConnectionRest').classList.remove('error')
  } else {
    document.querySelector('#serverConnectionRest').classList.add('error')
  }

  if (window.websocketAvailable) {
    document.querySelector('#serverConnectionWebSocket').classList.remove('error')
    document.querySelector('#serverConnectionWebSocket').innerText = 'WebSocket'
  } else {
    document.querySelector('#serverConnectionWebSocket').classList.add('error')
    document.querySelector('#serverConnectionWebSocket').innerText = 'WebSocket Error'
  }
}
