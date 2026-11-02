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
registerPlatformBrandNavigationTests(true)
registerPlatformOverviewTests(true)
registerBrandOverviewTests(true)
registerBrandSettingsTests(true)
registerSecuritySettingsTests(true)
registerActionEditorTests(true)
registerPlatformSettingsEditorTests(true)
registerMemberManagementTests(true)
registerPlatformMemberManagementTests(true)
registerAgentDirectoryTests(true)
registerCompactListTests(true)
registerBuiltinRoleTests()
registerManagementButtonTests()
registerControlSizingTests(true)

registerTemplateEditorTests(true)
