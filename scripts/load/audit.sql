-- Run in one snapshot; compare committed inventory, orders, claims and ledger deltas.
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
WITH ledger AS (
    SELECT ticket_type_id,
           sum(available_delta) AS available, sum(reserved_delta) AS reserved,
           sum(sold_delta) AS sold, sum(withheld_delta) AS withheld
    FROM inventory_events GROUP BY ticket_type_id
), order_totals AS (
    SELECT ticket_type_id,
           count(*) FILTER (WHERE state = 'pending') AS reserved,
           count(*) FILTER (WHERE state IN ('paid', 'refunding')) AS sold,
           count(*) FILTER (WHERE state = 'refunded') AS withheld
    FROM orders GROUP BY ticket_type_id
)
SELECT json_build_object(
    'orders', (SELECT count(*) FROM orders),
    'states', (SELECT json_object_agg(state, count) FROM (SELECT state, count(*) FROM orders GROUP BY state) s),
    'inventory', (SELECT json_agg(t) FROM ticket_types t),
    'negative_inventory', (SELECT count(*) FROM ticket_types WHERE least(available,reserved,sold,withheld) < 0),
    'ledger_mismatches', (SELECT count(*) FROM ticket_types t LEFT JOIN ledger l ON l.ticket_type_id=t.id
        WHERE ROW(t.available,t.reserved,t.sold,t.withheld) IS DISTINCT FROM ROW(l.available,l.reserved,l.sold,l.withheld)),
    'order_inventory_mismatches', (SELECT count(*) FROM ticket_types t LEFT JOIN order_totals o ON o.ticket_type_id=t.id
        WHERE t.reserved<>coalesce(o.reserved,0) OR t.sold<>coalesce(o.sold,0) OR t.withheld<>coalesce(o.withheld,0)),
    'duplicate_active_buyers', (SELECT count(*) FROM (SELECT product_id,buyer_id FROM orders
        WHERE state IN ('pending','paid','refunding') GROUP BY product_id,buyer_id HAVING count(*)>1) x),
    'duplicate_request_ids', (SELECT count(*) FROM (SELECT buyer_id,request_id FROM orders GROUP BY buyer_id,request_id HAVING count(*)>1) x),
    'claim_mismatches', (SELECT count(*) FROM purchase_claims c LEFT JOIN orders o ON o.id=c.order_id
        WHERE (c.order_id IS NOT NULL AND (o.id IS NULL OR o.buyer_id<>c.buyer_id OR o.product_id<>c.product_id))
           OR (c.permanent AND (o.id IS NULL OR o.state NOT IN ('paid','refunding','refunded')))
           OR (NOT c.permanent AND c.order_id IS NOT NULL AND o.state<>'pending')),
    'active_orders_without_claim', (SELECT count(*) FROM orders o LEFT JOIN purchase_claims c
        ON c.product_id=o.product_id AND c.buyer_id=o.buyer_id
        WHERE o.state IN ('pending','paid','refunding') AND (c.order_id IS DISTINCT FROM o.id)),
    'payment_mismatches', (SELECT count(*) FROM orders o LEFT JOIN simulated_payments p ON p.order_id=o.id
        WHERE (o.state IN ('paid','refunded') AND (p.order_id IS NULL OR p.closed OR p.reference IS DISTINCT FROM o.payment_reference
            OR p.amount<>o.price_cents OR p.refunded<>(o.state='refunded'))))
);
COMMIT;
