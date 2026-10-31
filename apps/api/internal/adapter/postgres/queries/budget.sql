-- name: ListExpenses :many
-- Newest day first.
SELECT * FROM expenses
WHERE household_id = @household_id AND day >= @from_day AND day < @to_day
ORDER BY day DESC, created_at DESC, id
LIMIT @lim;

-- name: ExpenseShares :many
SELECT * FROM expense_shares WHERE expense_id = ANY(@ids::uuid[]) ORDER BY expense_id, cents DESC, user_id;

-- name: LatestExpenseDay :one
SELECT day FROM expenses
WHERE household_id = @household_id AND day < @before_day
ORDER BY day DESC
LIMIT 1;

-- name: GetExpense :one
SELECT * FROM expenses WHERE id = @id AND household_id = @household_id;

-- name: CreateExpense :one
INSERT INTO expenses (household_id, kind, cents, category, note, paid_by, day, added_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateExpense :one
UPDATE expenses SET cents = @cents, category = @category, note = @note, paid_by = @paid_by, day = @day, updated_at = now()
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: AddExpenseShare :exec
INSERT INTO expense_shares (expense_id, user_id, cents) VALUES ($1, $2, $3);

-- name: ClearExpenseShares :exec
DELETE FROM expense_shares WHERE expense_id = $1;

-- name: DeleteExpense :execrows
DELETE FROM expenses WHERE id = $1 AND household_id = $2;

-- name: BudgetNets :many
-- What each person paid minus their shares, over everything the household has.
SELECT t.user_id, sum(t.cents)::bigint AS cents FROM (
    SELECT e.paid_by AS user_id, e.cents FROM expenses e WHERE e.household_id = @household_id
    UNION ALL
    SELECT s.user_id, -s.cents FROM expense_shares s JOIN expenses e ON e.id = s.expense_id WHERE e.household_id = @household_id
) t
GROUP BY t.user_id;

-- name: SetExpenseReceipt :one
UPDATE expenses SET receipt_at = @receipt_at
WHERE id = @id AND household_id = @household_id
RETURNING *;
