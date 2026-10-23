package httpapi

import "strings"

// Membership is selected once through its original acquiring node. EXISTS keeps
// each order once; wallet balances are aggregated separately from orders.
var agentMembersSQL = `WITH scoped AS (
 SELECT m.id,u.username,coalesce(u.phone,'') AS phone,m.display_name,m.status,m.joined_at,
 n.id AS agent_id,a.username AS agent_username,n.id=$2::uuid AS is_direct,
 (SELECT coalesce(sum(pb.points),0)::text FROM point_accounts pa
  JOIN point_buckets pb ON pb.brand_id=pa.brand_id AND pb.account_id=pa.id
  WHERE pa.brand_id=m.brand_id AND pa.brand_member_id=m.id AND pb.state='available') AS available_points
 FROM brand_members m JOIN global_users u ON u.id=m.global_user_id
 JOIN agent_nodes n ON n.brand_id=m.brand_id AND n.id::text=m.attribution_snapshot->>'agent_id'
 JOIN agent_accounts a ON a.brand_id=n.brand_id AND a.id=n.member_id
 WHERE m.brand_id=$1 AND (($6='direct' AND n.id=$2::uuid) OR ($6='line' AND $2::uuid=ANY(n.path)))
 AND ($5='' OR strpos(lower(concat_ws(' ',u.username,u.phone,m.display_name)),lower($5))>0)
), orders AS (` + strings.NewReplacer(
	"brand_member_id=$2", "EXISTS(SELECT 1 FROM scoped WHERE scoped.id=brand_member_id)",
	"member_id=$2", "EXISTS(SELECT 1 FROM scoped WHERE scoped.id=member_id)",
).Replace(memberOrderMetricsSQL) + `), page AS (
 SELECT * FROM scoped ORDER BY joined_at,id LIMIT $3 OFFSET $4
)
SELECT jsonb_build_object(
 'scope',$6::text,'q',$5::text,
 'items',coalesce((SELECT jsonb_agg(to_jsonb(page) ORDER BY joined_at,id) FROM page),'[]'::jsonb),
 'total_count',(SELECT count(*)::text FROM scoped),'limit',$3::int,'offset',$4::int,
 'summary',jsonb_build_object(
  'member_count',(SELECT count(*)::text FROM scoped),
  'available_points',(SELECT coalesce(sum(available_points::numeric),0)::text FROM scoped),
  'bet_count',(SELECT count(*)::text FROM orders),
  'stake_points',(SELECT coalesce(sum(stake),0)::text FROM orders),
  'unsettled_points',(SELECT coalesce(sum(stake) FILTER(WHERE pending),0)::text FROM orders),
  'net_points',(SELECT coalesce(sum(net) FILTER(WHERE settled_at IS NOT NULL),0)::text FROM orders)
 ))`
