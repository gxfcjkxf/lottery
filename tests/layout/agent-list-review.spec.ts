import { test, expect } from '@playwright/test';
import { gameId, origins, installLayoutApi, expectNoPageOverflow } from './fixture';
for (const locale of ['en', 'zh-CN'] as const)
    test(`agent full pages share toolbar and pagination (${locale})`, async ({ page }, info) => {
        const zh = locale === 'zh-CN', at = '2026-11-02T00:00:00Z', reads: URL[] = [], errors: string[] = [];
        await installLayoutApi(page, locale);
        page.on('pageerror', e => errors.push(e.message));
        let failMembers = false, emptyRecords = false, holdMembers = false, releaseMembers: () => void = () => { throw Error('Member read has not reached the test gate'); };
        const members = Array.from({ length: 41 }, (_, i) => ({ id: `bbbbbbbb-bbbb-4bbb-8bbb-${String(i + 1).padStart(12, '0')}`, username: `member_${i}`, display_name: '', phone: '', available_points: '9007199254740993', agent_username: 'review_agent', is_direct: true, joined_at: at, status: 'active' }));
        await page.route('**/api/v1/agent/**', async (route) => {
            const req = route.request(), url = new URL(req.url()), path = url.pathname;
            expect(req.method()).toBe('GET');
            reads.push(url);
            const reply = (data: unknown) => route.fulfill({ json: { success: true, data, associations: { [`agent:${gameId}`]: { name: 'child_reviewer' } } } });
            if (path.endsWith('/me'))
                return reply({ account: { agent_account_id: gameId, username: 'review_agent' }, agent: { id: gameId, brand_id: gameId, depth: 1, config: { can_create_children: true } }, code: null, user_origin: origins.user });
            if (path.endsWith('/members')) {
                if (failMembers) {
                    failMembers = false;
                    return route.fulfill({ status: 503, json: { success: false, error: { code: 'SERVICE_UNAVAILABLE', message: 'Synthetic read failure' } } });
                }
                if (holdMembers) {
                    holdMembers = false;
                    await new Promise<void>(resolve => { releaseMembers = resolve; });
                }
                const q = url.searchParams.get('q'), items = members.filter(m => !q || m.username === q), offset = Number(url.searchParams.get('offset'));
                return reply({ items: items.slice(offset, offset + 20), total_count: String(items.length), offset, limit: 20, summary: { member_count: String(items.length), available_points: '9007199254740993', bet_count: '0', stake_points: '0', unsettled_points: '0', net_points: '0' } });
            }
            if (path.endsWith('/children'))
                return reply({ items: emptyRecords ? [] : [{ id: gameId, depth: 2, config: { status: 'active' } }], total_count: emptyRecords ? '0' : '1', offset: 0, limit: 20 });
            if (path.endsWith('/wallet'))
                return reply({ available_points: '10', commission_points: '10' });
            if (path.endsWith('/commission'))
                return reply({ items: emptyRecords ? [] : [{ id: gameId, created_at: at, entry_type: 'commission_credit', points: '10' }], total_count: emptyRecords ? '0' : '1', offset: 0, limit: 20, credited_points: '10', debited_points: '0' });
            if (path.endsWith('/summary'))
                return reply({ member: members[0], wallet: { available_points: '9007199254740993' }, net_points: '0', unsettled_points: '0' });
            if (path.endsWith('/bets'))
                return reply({ items: [], total_count: '0', next_cursor: null });
            throw Error('Unexpected agent read ' + path);
        });
        await page.goto(origins.agent);
        const list = page.locator('.member-directory'), pager = list.locator('.table-pagination');
        await expect(list.locator('tbody tr')).toHaveCount(20);
        const preset = page.viewportSize()!;
        async function review(label: string) {
            for (const width of info.project.name.endsWith('reflow') ? [320, 640, 900, 1080, 1440] : [preset.width]) {
                if (info.project.name.endsWith('reflow'))
                    await page.setViewportSize({ width, height: preset.height });
                await expectNoPageOverflow(page);
                const activeList = page.locator('.management-list'), pagination = activeList.locator('.table-pagination');
                await expect(activeList).toHaveCount(1);
                await expect(pagination).toHaveCount(label === 'bets' ? 0 : 1);
                await expect(activeList.locator('.management-list')).toHaveCount(0);
                for (const value of await activeList.locator('.management-list-summary dd').all()) {
                    expect(await value.evaluate(el => el.getBoundingClientRect().height <= parseFloat(getComputedStyle(el).lineHeight) + 1)).toBe(true);
                }
                const query = activeList.locator('.management-list-filters button[type=submit]'), actions = activeList.locator('.management-list-toolbar > .console-page-actions');
                if (width >= 1080 && await query.count() && await actions.count()) {
                    const q = await query.boundingBox(), a = await actions.boundingBox();
                    expect(Math.abs(q!.y + q!.height - a!.y - a!.height)).toBeLessThanOrEqual(2);
                }
                if (await pagination.count()) {
                    await expect(activeList.locator(':scope > .table-pagination')).toHaveCount(1);
                    await pagination.scrollIntoViewIfNeeded();
                    await expect(pagination.locator('.table-pagination-arrow')).toHaveText(['‹', '›']);
                    await expect(pagination.locator('.table-pagination-jump label')).toHaveCSS('flex-direction', 'row');
                }
                await page.evaluate(() => scrollTo(0, 0));
                await page.screenshot({ path: info.outputPath(`${label}-${locale}-${width}.png`), fullPage: true });
            }
            await page.setViewportSize(preset);
        }
        await review('members');
        const toolbar = list.locator('.management-list-toolbar'), add = list.getByRole('button', { name: zh ? '添加会员' : 'Add member', exact: true });
        await expect(toolbar.locator('.console-page-actions')).toContainText(zh ? '添加会员' : 'Add member');
        await add.click();
        const create = page.getByRole('dialog', { name: zh ? '添加会员' : 'Add member', exact: true });
        await expect(create).toBeVisible();
        await expect(create.getByLabel(zh ? '账号' : 'Account', { exact: true })).toBeVisible();
        await expect(create.locator('button[type=submit]')).toBeDisabled();
        await create.locator('.detail-modal-close').click();
        await expect(list.locator('tbody tr')).toHaveCount(20);
        if (preset.width >= 1080) {
            const a = await add.boundingBox(), q = await list.locator('button[type=submit]').boundingBox();
            expect(Math.abs(a!.y - q!.y)).toBeLessThanOrEqual(2);
        }
        failMembers = true;
        await pager.getByRole('button', { name: zh ? '下一页' : 'Next', exact: true }).click();
        await expect(page.locator('.toast-error')).toBeVisible();
        await expect(pager.locator('[aria-current=page]')).toHaveText('1');
        await expect(list.locator('tbody tr').first()).toContainText('member_0');
        await page.locator('.toast-error .toast-close').click();
        holdMembers = true;
        await pager.getByRole('button', { name: zh ? '下一页' : 'Next', exact: true }).click();
        try {
            await expect(pager.locator('.table-pagination-arrow').last()).toBeDisabled();
            await expect(pager.getByRole('textbox')).toBeDisabled();
            await expect(pager.locator('[aria-current=page]')).toHaveText('1');
            await expect(list.locator('tbody tr').first()).toContainText('member_0');
        }
        finally {
            releaseMembers();
        }
        await expect(pager.locator('[aria-current=page]')).toHaveText('2');
        await pager.getByRole('textbox').fill('3');
        await pager.getByRole('button', { name: 'GO', exact: true }).click();
        await expect(list.locator('tbody tr')).toHaveCount(1);
        expect(reads.filter(url => url.pathname.endsWith('/members')).at(-1)!.searchParams.get('offset')).toBe('40');
        const search = list.getByRole('textbox', { name: zh ? '账号' : 'Account', exact: true });
        await search.fill('missing');
        await search.press('Enter');
        await expect(list.locator('td.empty-state')).toBeVisible();
        await expect(pager).toHaveCount(0);
        await list.getByRole('button', { name: zh ? '清空' : 'Clear', exact: true }).click();
        await expect(list.locator('tbody tr')).toHaveCount(20);
        await expect(list.locator('tbody tr').first().locator('td').nth(1)).toHaveText('—');
        await expect(list.locator('.member-menu-trigger').first()).toHaveCSS('border-top-width', '0px');
        await list.locator('.member-menu-trigger').first().click();
        await page.getByRole('menuitem', { name: zh ? '查看注单' : 'View bets', exact: true }).click();
        await expect(page.locator('.member-bets')).toContainText('member_0');
        await review('bets');
        await page.getByRole('button', { name: zh ? '选择会员' : 'Select member', exact: true }).click();
        await expect(list).toBeVisible();
        for (const id of ['children', 'commission']) {
            await page.goto(origins.agent + '/#/' + id);
            await expect(page.locator('.management-list tbody tr')).toHaveCount(1);
            await review(id);
            emptyRecords = true;
            await page.locator('.management-list-toolbar [data-refresh-button]').click();
            await expect(page.locator('td.empty-state')).toBeVisible();
            await expect(page.locator('.management-list thead')).toHaveCount(1);
            await expect(page.locator('.table-pagination')).toHaveCount(0);
            emptyRecords = false;
        }
        expect(errors).toEqual([]);
    });
