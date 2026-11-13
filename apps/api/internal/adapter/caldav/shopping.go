package caldav

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// The shopping list is one more reminders list, shared by the household: an item is a to-do
// that is open or ticked off, with nothing but its text.

func (b *backend) shoppingObject(ctx context.Context, l list, it domain.ShoppingItem) (*caldav.CalendarObject, error) {
	home, _ := b.CalendarHomeSetPath(ctx)
	cal, data, err := shoppingToICal(it)
	if err != nil {
		return nil, err
	}
	return &caldav.CalendarObject{
		Path:          home + l.slug + "/" + it.SyncName(),
		ModTime:       it.Created,
		ContentLength: int64(len(data)),
		ETag:          etag(data),
		Data:          cal,
	}, nil
}

func shoppingToICal(it domain.ShoppingItem) (*ical.Calendar, []byte, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Homeloom//Homeloom//EN")

	comp := ical.NewComponent(ical.CompToDo)
	uid := it.ICalUID
	if uid == "" {
		uid = it.ID.String() + "@homeloom"
	}
	comp.Props.SetText(ical.PropUID, uid)
	// Nothing about an item changes after it is made except being ticked off, so these are enough
	// for an app to see a change: the ETag is a hash of the whole object.
	stamp := it.Created.UTC()
	if it.DoneAt != nil {
		stamp = it.DoneAt.UTC()
	}
	comp.Props.SetDateTime(ical.PropDateTimeStamp, stamp)
	comp.Props.SetDateTime(ical.PropCreated, it.Created.UTC())
	comp.Props.SetDateTime(ical.PropLastModified, stamp)
	comp.Props.SetText(ical.PropSummary, it.Text)
	if it.Recipe != "" {
		comp.Props.SetText(ical.PropDescription, it.Recipe)
	}
	if it.DoneAt != nil {
		comp.Props.SetText(ical.PropStatus, "COMPLETED")
		comp.Props.SetDateTime(ical.PropCompleted, it.DoneAt.UTC())
	} else {
		comp.Props.SetText(ical.PropStatus, "NEEDS-ACTION")
	}
	cal.Children = append(cal.Children, comp)

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, nil, fmt.Errorf("encode shopping item %s: %w", it.ID, err)
	}
	return cal, buf.Bytes(), nil
}

func (b *backend) shoppingObjects(ctx context.Context, l list) ([]caldav.CalendarObject, error) {
	g := grant(ctx)
	items, err := b.Shopping.SyncItems(ctx, g.User, g.HouseholdID)
	if err != nil {
		return nil, httpError(ctx, err)
	}
	out := make([]caldav.CalendarObject, 0, len(items))
	for _, it := range items {
		o, err := b.shoppingObject(ctx, l, it)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, nil
}

func (b *backend) shoppingGet(ctx context.Context, l list, name string) (*caldav.CalendarObject, error) {
	g := grant(ctx)
	it, err := b.Shopping.SyncItem(ctx, g.User, g.HouseholdID, name)
	if err != nil {
		return nil, httpError(ctx, err)
	}
	return b.shoppingObject(ctx, l, it)
}

func (b *backend) shoppingPut(ctx context.Context, l list, name string, cal *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	kind, uid, err := caldav.ValidateCalendarObject(cal)
	if err != nil {
		return nil, caldav.NewPreconditionError(caldav.PreconditionValidCalendarObjectResource)
	}
	if kind != l.comp {
		return nil, caldav.NewPreconditionError(caldav.PreconditionSupportedCalendarComponent)
	}
	in, err := fromICal(master(cal, kind), domain.KindReminder, b.Location)
	if err != nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	g := grant(ctx)
	if opts.IfMatch.IsSet() || opts.IfNoneMatch.IsSet() {
		var tag string
		cur, err := b.Shopping.SyncItem(ctx, g.User, g.HouseholdID, name)
		switch {
		case err == nil:
			o, err := b.shoppingObject(ctx, l, cur)
			if err != nil {
				return nil, err
			}
			tag = o.ETag
		case !errors.Is(err, domain.ErrNotFound):
			return nil, httpError(ctx, err)
		}
		if err := conditions(tag, opts); err != nil {
			return nil, err
		}
	}
	it, err := b.Shopping.SyncPut(ctx, g.User, g.HouseholdID, name, uid, in.Text, in.Done)
	if err != nil {
		return nil, httpError(ctx, err)
	}
	return b.shoppingObject(ctx, l, it)
}
