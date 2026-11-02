import {registerPlatformBrandNavigationTests} from './platform-brand-navigation'
import {registerPlatformOverviewTests} from './platform-overview'
import {registerBrandOverviewTests} from './brand-overview'
import {registerBrandSettingsTests} from './brand-settings'
import {registerSecuritySettingsTests} from './security-settings'
import {registerActionEditorTests} from './action-editors'
import {registerPlatformSettingsEditorTests,registerTemplateEditorTests} from './platform-settings'
import {registerMemberManagementTests} from './member-management'
import {registerPlatformMemberManagementTests} from './platform-member-management'
import {registerAgentDirectoryTests} from './agent-directory'
import {registerCompactListTests} from './compact-lists'
import {registerBuiltinRoleTests} from './builtin-roles'
import {registerManagementButtonTests} from './management-buttons'
import {registerControlSizingTests} from './control-sizing'
registerPlatformBrandNavigationTests(false)
registerPlatformOverviewTests(false)
registerBrandOverviewTests(false)
registerBrandSettingsTests(false)
registerSecuritySettingsTests(false)
registerActionEditorTests(false)
registerPlatformSettingsEditorTests(false)
registerMemberManagementTests(false)
registerPlatformMemberManagementTests(false)
registerAgentDirectoryTests(false)
registerCompactListTests(false)
registerBuiltinRoleTests()
registerManagementButtonTests()
registerControlSizingTests(false)

registerTemplateEditorTests(false)
