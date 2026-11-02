import { registerControlSizingTests } from './control-sizing'
import './dropdown-interactions.spec'
import { registerPlatformSettingsEditorTests } from './platform-settings'
import { registerPlatformMemberManagementTests } from './platform-member-management'
import { registerAgentDirectoryTests } from './agent-directory'
import { registerFormValidationTests } from './form-validation'
import './results-fixed.spec'
registerPlatformSettingsEditorTests()
registerPlatformMemberManagementTests()
registerAgentDirectoryTests()
registerFormValidationTests()

registerControlSizingTests()
