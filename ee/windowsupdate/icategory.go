package windowsupdate

import (
	"fmt"

	"github.com/go-ole/go-ole"
)

// ICategory represents the category to which an update belongs.
// https://docs.microsoft.com/en-us/windows/win32/api/wuapi/nn-wuapi-icategory
type ICategory struct {
	CategoryID  string
	Children    []*ICategory
	Description string
	Image       *IImageInformation
	Name        string
	Order       int32
	Parent      *ICategory
	Type        string
	Updates     []*IUpdate
}

func toICategories(categoriesDisp *ole.IDispatch) ([]*ICategory, error) {
	count, err := getPropertyInt32(categoriesDisp, "Count")
	if err != nil {
		return nil, fmt.Errorf("Count: %w", err)
	}

	categories := make([]*ICategory, 0, count)
	for i := 0; i < int(count); i++ {
		categoryDisp, err := getPropertyDispatch(categoriesDisp, "Item", i)
		if err != nil {
			return nil, fmt.Errorf("Item[%d/%d]: %w", i, count, err)
		}

		category, err := toICategory(categoryDisp)
		if err != nil {
			return nil, fmt.Errorf("converting Item[%d/%d]: %w", i, count, err)
		}

		categories = append(categories, category)
	}
	return categories, nil
}

func toICategory(categoryDisp *ole.IDispatch) (*ICategory, error) {
	defer categoryDisp.Release()

	var err error
	iCategory := &ICategory{}

	if iCategory.CategoryID, err = getPropertyString(categoryDisp, "CategoryID"); err != nil {
		return nil, fmt.Errorf("CategoryID: %w", err)
	}

	if childrenDisp, err := getPropertyDispatch(categoryDisp, "Children"); err != nil {
		return nil, fmt.Errorf("Children: %w", err)
	} else if childrenDisp != nil {
		defer childrenDisp.Release()
		if iCategory.Children, err = toICategories(childrenDisp); err != nil {
			return nil, fmt.Errorf("converting Children: %w", err)
		}
	}

	if iCategory.Description, err = getPropertyString(categoryDisp, "Description"); err != nil {
		return nil, fmt.Errorf("Description: %w", err)
	}

	if imageDisp, err := getPropertyDispatch(categoryDisp, "Image"); err != nil {
		return nil, fmt.Errorf("Image: %w", err)
	} else if imageDisp != nil {
		defer imageDisp.Release()
		if iCategory.Image, err = toIImageInformation(imageDisp); err != nil {
			return nil, fmt.Errorf("converting Image: %w", err)
		}
	}

	if iCategory.Name, err = getPropertyString(categoryDisp, "Name"); err != nil {
		return nil, fmt.Errorf("Name: %w", err)
	}

	if iCategory.Order, err = getPropertyInt32(categoryDisp, "Order"); err != nil {
		return nil, fmt.Errorf("Order: %w", err)
	}

	// Parent is commented out to avoid infinite recursion (Parent -> Category -> Parent ...)
	// See original code.

	if iCategory.Type, err = getPropertyString(categoryDisp, "Type"); err != nil {
		return nil, fmt.Errorf("Type: %w", err)
	}

	// Updates is commented out to avoid pulling the full update tree per category.
	// See original code.

	return iCategory, nil
}
